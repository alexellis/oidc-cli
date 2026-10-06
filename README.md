# oidc-cli

A small CLI that performs a full OpenID Connect Authorization Code flow with
[PKCE](https://datatracker.ietf.org/doc/html/rfc7636) against a
[Signet](https://github.com/openfaasltd/signet) instance, and prints the
resulting tokens and ID token claims.

Signet has no headless end-user login helper: its CLI (`signet`) manages
users and clients through the admin API, but nothing performs an actual
end-user OAuth login for scripts and e2e tests. This tool fills that gap.

## How it works

1. `GET /authorize` — fetches the login form and extracts the form action
2. `POST` username + password — intercepts the redirect to the registered
   callback URL and captures the authorization `code` and `state`
   (state is validated against the value sent in step 1)
3. `POST /token` — exchanges the code with client basic auth and the
   PKCE `code_verifier`
4. Prints the token response and decodes the ID token claims

Note: step 2 scrapes the HTML login form, which is an undocumented contract
of the Signet UI and may break if the UI changes.

## Requirements

* A running Signet instance (v0.0.9 was used for testing)
* A confidential client registered with a redirect URL of
  `http://127.0.0.1:<port>/callback`
* Go 1.22+ to build

## Install

```bash
go install github.com/alexellis/oidc-cli@latest
```

Or clone and build:

```bash
git clone https://github.com/alexellis/oidc-cli
cd oidc-cli
go build -o oidc-cli .
sudo cp oidc-cli /usr/local/bin/
```

## Usage

Create a user and client with the Signet CLI, saving the generated secret:

```bash
export SIGNET_URL=https://signet.example.com
export SIGNET_ADMIN_TOKEN=$(kubectl -n openfaas get secret signet-admin-token \
  -o jsonpath='{.data.admin-token}' | base64 -d)

signet user add --groups admin alex
signet client add --redirect-url http://127.0.0.1:9999/callback my-cli
```

Then run the flow:

```bash
oidc-cli \
  --issuer https://signet.example.com \
  --client-id my-cli \
  --client-secret <secret> \
  --username alex \
  --password <password>
```

Example output:

```json
{
  "access_token": "eyJhbGciOi...",
  "expires_in": 3600,
  "id_token": "eyJhbGciOi...",
  "token_type": "Bearer"
}

ID token claims:
{
  "aud": ["my-cli"],
  "groups": ["admin"],
  "iss": "https://signet.example.com",
  "sub": "alex"
}
```

Exit codes are non-zero on any failure, including wrong credentials
(login rejected) or a bad client secret (`400 invalid_client`).

## License

MIT
