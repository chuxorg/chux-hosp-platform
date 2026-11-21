package auth

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Identity struct {
	Issuer  string
	Subject string
	Nonce   string
}

type IdentityProvider interface {
	AuthorizationURL(state, nonce, challenge string) string
	Exchange(context.Context, string, string) (Identity, error)
	LogoutURL(string) (string, error)
}

type OIDCProvider struct {
	issuer     string
	clientID   string
	endSession string
	oauth      oauth2.Config
	verifier   *oidc.IDTokenVerifier
}

func NewOIDCProvider(ctx context.Context, issuer, clientID, clientSecret, redirectURL string) (*OIDCProvider, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, errors.New("OIDC discovery failed")
	}
	var metadata struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return nil, errors.New("OIDC metadata is invalid")
	}
	if metadata.EndSessionEndpoint == "" {
		metadata.EndSessionEndpoint = strings.TrimRight(issuer, "/") + "/v2/logout"
	}
	return &OIDCProvider{
		issuer:     issuer,
		clientID:   clientID,
		endSession: metadata.EndSessionEndpoint,
		oauth:      oauth2.Config{ClientID: clientID, ClientSecret: clientSecret, Endpoint: provider.Endpoint(), RedirectURL: redirectURL, Scopes: []string{oidc.ScopeOpenID, "profile"}},
		verifier:   provider.Verifier(&oidc.Config{ClientID: clientID}),
	}, nil
}

func (p *OIDCProvider) AuthorizationURL(state, nonce, challenge string) string {
	return p.oauth.AuthCodeURL(state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

func (p *OIDCProvider) Exchange(ctx context.Context, code, verifier string) (Identity, error) {
	token, err := p.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, errors.New("OIDC code exchange failed")
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok || raw == "" {
		return Identity{}, errors.New("OIDC response omitted ID token")
	}
	idToken, err := p.verifier.Verify(ctx, raw)
	if err != nil {
		return Identity{}, errors.New("OIDC ID token verification failed")
	}
	var claims struct {
		Issuer  string `json:"iss"`
		Subject string `json:"sub"`
		Nonce   string `json:"nonce"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Issuer != p.issuer || claims.Subject == "" || claims.Nonce == "" {
		return Identity{}, errors.New("OIDC identity claims are invalid")
	}
	return Identity{Issuer: claims.Issuer, Subject: claims.Subject, Nonce: claims.Nonce}, nil
}

func (p *OIDCProvider) LogoutURL(returnTo string) (string, error) {
	if p.endSession == "" {
		return "", errors.New("OIDC provider omitted logout endpoint")
	}
	u, err := url.Parse(p.endSession)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", errors.New("OIDC logout endpoint is invalid")
	}
	q := u.Query()
	q.Set("client_id", p.clientID)
	q.Set("returnTo", returnTo)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
