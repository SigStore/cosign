package fulcio

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"os"

	"github.com/sigstore/fulcio/cmd/client/app"

	"github.com/coreos/go-oidc"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/sigstore/fulcio/pkg/generated/client/operations"
	"github.com/sigstore/fulcio/pkg/generated/models"
	"github.com/sigstore/fulcio/pkg/oauthflow"
	"golang.org/x/oauth2"
)

const defaultFulcioAddress = "https://fulcio.sigstore.dev"

func fulcioServer() string {
	addr := os.Getenv("FULCIO_ADDRESS")
	if addr != "" {
		return addr
	}
	return defaultFulcioAddress
}

func GetCert(ctx context.Context, priv *ecdsa.PrivateKey) (string, error) {
	fcli, err := app.GetFulcioClient(fulcioServer())
	if err != nil {
		return "", err
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return "", err
	}
	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return "", err
	}

	// TODO: Switch these to be creds from the sigstore project.
	config := oauth2.Config{
		ClientID: "237800849078-rmntmr1b2tcu20kpid66q5dbh1vdt7aj.apps.googleusercontent.com",
		// THIS IS NOT A SECRET - IT IS USED IN THE NATIVE/DESKTOP FLOW.
		ClientSecret: "CkkuDoCgE2D_CCRRMyF_UIhS",
		Endpoint:     provider.Endpoint(),
		RedirectURL:  "http://127.0.0.1:5556/auth/google/callback",
		Scopes:       []string{oidc.ScopeOpenID, "email"},
	}

	accessToken, err := oauthflow.GetAccessToken(provider, config)
	if err != nil {
		return "", err
	}

	userInfo, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: accessToken,
	}))
	if err != nil {
		return "", err
	}

	// Sign the email address as part of the request
	h := sha256.Sum256([]byte(userInfo.Email))
	proof, err := ecdsa.SignASN1(rand.Reader, priv, h[:])
	if err != nil {
		return "", err
	}

	apiKeyQueryAuth := httptransport.APIKeyAuth("X-Access-Token", "header", accessToken)

	params := operations.NewSigningCertParams()
	params.SetSubmitcsr(&models.Submit{
		Pub:   strfmt.Base64(base64.StdEncoding.EncodeToString(pubBytes)),
		Proof: strfmt.Base64(base64.StdEncoding.EncodeToString(proof)),
	})
	resp, err := fcli.Operations.SigningCert(params, apiKeyQueryAuth)
	if err != nil {
		return "", err
	}
	return resp.Payload.Certificate, nil
}

// This is the root in the fulcio project.
const rootPem = `-----BEGIN CERTIFICATE-----
MIIB+DCCAX6gAwIBAgITNVkDZoCiofPDsy7dfm6geLbuhzAKBggqhkjOPQQDAzAq
MRUwEwYDVQQKEwxzaWdzdG9yZS5kZXYxETAPBgNVBAMTCHNpZ3N0b3JlMB4XDTIx
MDMwNzAzMjAyOVoXDTMxMDIyMzAzMjAyOVowKjEVMBMGA1UEChMMc2lnc3RvcmUu
ZGV2MREwDwYDVQQDEwhzaWdzdG9yZTB2MBAGByqGSM49AgEGBSuBBAAiA2IABLSy
A7Ii5k+pNO8ZEWY0ylemWDowOkNa3kL+GZE5Z5GWehL9/A9bRNA3RbrsZ5i0Jcas
taRL7Sp5fp/jD5dxqc/UdTVnlvS16an+2Yfswe/QuLolRUCrcOE2+2iA5+tzd6Nm
MGQwDgYDVR0PAQH/BAQDAgEGMBIGA1UdEwEB/wQIMAYBAf8CAQEwHQYDVR0OBBYE
FMjFHQBBmiQpMlEk6w2uSu1KBtPsMB8GA1UdIwQYMBaAFMjFHQBBmiQpMlEk6w2u
Su1KBtPsMAoGCCqGSM49BAMDA2gAMGUCMH8liWJfMui6vXXBhjDgY4MwslmN/TJx
Ve/83WrFomwmNf056y1X48F9c4m3a3ozXAIxAKjRay5/aj/jsKKGIkmQatjI8uup
Hr/+CxFvaJWmpYqNkLDGRU+9orzh5hI2RrcuaQ==
-----END CERTIFICATE-----`

var Roots *x509.CertPool

func init() {
	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM([]byte(rootPem)) {
		panic("error creating root cert pool")
	}
	Roots = cp
}
