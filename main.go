package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"oidc-cli/pkg"
)

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("oidc-cli version %s", pkg.BuildString())
		if pkg.BuildDateString() != "" {
			fmt.Printf(" (built %s)", pkg.BuildDateString())
		}
		if pkg.GitCommit != "" {
			fmt.Printf(", commit: %s", pkg.GitCommit)
		}
		fmt.Println()
		return
	}

	issuer := flag.String("issuer", "", "Signet issuer base URL (required)")
	clientID := flag.String("client-id", "", "OAuth client id")
	clientSecret := flag.String("client-secret", "", "OAuth client secret")
	username := flag.String("username", "", "Signet username")
	password := flag.String("password", "", "Signet password")
	port := flag.Int("redirect-port", 9999, "port matching the client's registered redirect URL")
	flag.Parse()

	if *issuer == "" || *clientID == "" || *username == "" || *password == "" {
		flag.Usage()
		os.Exit(1)
	}
	*issuer = strings.TrimRight(*issuer, "/")
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", *port)

	verifier := randomString(48)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	state := randomString(16)

	authURL := fmt.Sprintf("%s/authorize?%s", *issuer, url.Values{
		"client_id":             {*clientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {"openid profile email"},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode())

	client := &http.Client{Timeout: 30 * time.Second}
	client.Transport = userAgentTransport{pkg.UserAgent(), http.DefaultTransport}

	// 1. GET the login form
	resp, err := client.Get(authURL)
	if err != nil {
		fatal("GET /authorize: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	formAction := extractFormAction(string(body))
	if formAction == "" {
		fatal("no login form found at %s", authURL)
	}
	if !strings.HasPrefix(formAction, "http") {
		formAction = *issuer + formAction
	}

	// 2. POST credentials, do not follow the redirect to the callback
	form := url.Values{"username": {*username}, "password": {*password}}
	code, cbState, err := exchangeForm(client, formAction, form, redirectURI)
	if err != nil {
		fatal("login: %v", err)
	}
	if cbState != state {
		fatal("state mismatch: got %q want %q", cbState, state)
	}

	// 3. Exchange the code at /token (PKCE + secret)
	tokenURL := *issuer + "/token"
	tr := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	req, _ := http.NewRequest("POST", tokenURL, strings.NewReader(tr.Encode()))
	req.SetBasicAuth(*clientID, *clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = client.Do(req)
	if err != nil {
		fatal("POST /token: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		fatal("token exchange failed (%d): %s", resp.StatusCode, body)
	}
	var tok TokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		fatal("parse token response: %v", err)
	}

	out, _ := json.MarshalIndent(map[string]any{
		"token_type":    tok.TokenType,
		"expires_in":    tok.ExpiresIn,
		"scope":         tok.Scope,
		"access_token":  tok.AccessToken,
		"id_token":      tok.IDToken,
		"refresh_token": tok.RefreshToken,
	}, "", "  ")
	fmt.Println(string(out))

	// 4. Decode and print ID token claims
	if tok.IDToken != "" {
		if claims, err := decodeJWT(tok.IDToken); err == nil {
			fmt.Println("\nID token claims:")
			c, _ := json.MarshalIndent(claims, "", "  ")
			fmt.Println(string(c))
		}
	}
}

// exchangeForm posts the login form and, when the server redirects to the
// callback, intercepts it to capture the authorization code.
func exchangeForm(client *http.Client, action string, form url.Values, redirectURI string) (string, string, error) {
	redirClient := &http.Client{
		Transport: userAgentTransport{pkg.UserAgent(), http.DefaultTransport},
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if strings.HasPrefix(req.URL.String(), redirectURI) {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	req, _ := http.NewRequest("POST", action, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := redirClient.Do(req)
	if err != nil {
		return "", "", err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode == 200 {
		return "", "", fmt.Errorf("login rejected (%s): %s", resp.Status, strings.TrimSpace(stripTags(string(body))))
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		u, err := url.Parse(loc)
		if err != nil {
			return "", "", err
		}
		if !strings.HasPrefix(loc, redirectURI) {
			return "", "", fmt.Errorf("unexpected redirect target: %s", loc)
		}
		return u.Query().Get("code"), u.Query().Get("state"), nil
	}
	return "", "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
}

func extractFormAction(body string) string {
	idx := strings.Index(body, "<form")
	if idx < 0 {
		return ""
	}
	seg := body[idx:]
	actIdx := strings.Index(seg, `action="`)
	if actIdx < 0 {
		return ""
	}
	seg = seg[actIdx+len(`action="`):]
	end := strings.Index(seg, `"`)
	return html.UnescapeString(seg[:end])
}

func stripTags(s string) string {
	var out strings.Builder
	in := false
	for _, r := range s {
		if r == '<' {
			in = true
		} else if r == '>' {
			in = false
		} else if !in {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func decodeJWT(tok string) (map[string]any, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(payload, &m)
}

type userAgentTransport struct {
	ua string
	rt http.RoundTripper
}

func (u userAgentTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("User-Agent", u.ua)
	return u.rt.RoundTrip(r)
}

func randomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		fatal("rand: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func fatal(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "oidc-cli: "+f+"\n", a...)
	bufio.NewReader(os.Stdin)
	os.Exit(1)
}
