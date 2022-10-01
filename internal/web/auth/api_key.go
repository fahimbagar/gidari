// Copyright 2022 The Gidari Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// apiKeyTimestmapBase is the base time for calculating the timestamp parameter.
	apiKeyTimestampBase = 10
)

// APIKey is transport for authenticating with an API KEy. API Key authentication should only be used to access your
// own account. If your application requires access to other accounts, do not use API Key. API key authentication
// requires each request to be signed (enhanced security measure). Your API keys should be assigned to access only
// accounts and permission scopes that are necessary for your app to function.
type APIKey struct {
	key        string
	passphrase string
	secret     string
	url        *url.URL
}

// NewAPIKey will return an APIKey authentication transport.
func NewAPIKey() *APIKey {
	return new(APIKey)
}

// SetKey will set the key field on APIKey.
func (auth *APIKey) SetKey(key string) *APIKey {
	auth.key = key

	return auth
}

// SetPassphrase will set the key field on APIKey.
func (auth *APIKey) SetPassphrase(passphrase string) *APIKey {
	auth.passphrase = passphrase

	return auth
}

// SetSecret will set the key field on APIKey.
func (auth *APIKey) SetSecret(secret string) *APIKey {
	auth.secret = secret

	return auth
}

// SetURL will set the key field on APIKey.
func (auth *APIKey) SetURL(u string) *APIKey {
	auth.url, _ = url.Parse(u)

	return auth
}

// generateSig generates the coinbase base64-encoded signature required to make requests.  In particular, the
// c-ACCESS-SIGN header is generated by creating a sha256 HMAC using the base64-decoded secret key on the prehash string
// timestamp + method + requestPath + body (where + represents string concatenation) and base64-encode the output. The
// timestamp value is the same as the c-ACCESS-TIMESTAMP header.
func (auth *APIKey) generateSig(message string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(auth.secret)
	if err != nil {
		return "", fmt.Errorf("error decoding secret: %w", err)
	}

	signature := hmac.New(sha256.New, key)

	_, err = signature.Write([]byte(message))
	if err != nil {
		return "", fmt.Errorf("error writing signature: %w", err)
	}

	return base64.StdEncoding.EncodeToString(signature.Sum(nil)), nil
}

// bytes will return the byte stream for the body.
func parsebytes(req *http.Request) []byte {
	if req.Body == nil {
		return []byte{}
	}

	// Have to read the body from the request
	body, _ := io.ReadAll(req.Body)

	// And now set a new body, which will simulate the same data we read:
	req.Body = io.NopCloser(bytes.NewBuffer(body))

	return body
}

// generageMsg makes the message to be signed.
func (auth *APIKey) generageMsg(req *http.Request, timestamp string) string {
	postAuthority := strings.Replace(req.URL.String(), auth.url.String(), "", 1)

	return fmt.Sprintf("%s%s%s%s", timestamp, req.Method, postAuthority, string(parsebytes(req)))
}

// RoundTrip authorizes the request with a signed API Key Authorization header.
func (auth *APIKey) RoundTrip(req *http.Request) (*http.Response, error) {
	if auth.url == nil {
		return nil, ErrURLRequired
	}

	var (
		timestamp = strconv.FormatInt(time.Now().Unix(), apiKeyTimestampBase)
		msg       = auth.generageMsg(req, timestamp)
	)

	sig, err := auth.generateSig(msg)
	if err != nil {
		return nil, err
	}

	req.URL.Scheme = auth.url.Scheme
	req.URL.Host = auth.url.Host

	req.Header.Set("content-type", "application/json")
	req.Header.Add("cb-access-key", auth.key)
	req.Header.Add("cb-access-passphrase", auth.passphrase)
	req.Header.Add("cb-access-sign", sig)
	req.Header.Add("cb-access-timestamp", timestamp)

	rsp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}

	return rsp, nil
}
