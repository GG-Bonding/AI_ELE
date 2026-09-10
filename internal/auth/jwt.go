package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// JWTProvider validates HS256 Bearer tokens (V3.4.1).
// Expected claims: tenant_id (or tid), sub|actor_id, optional roles[], permissions[].
type JWTProvider struct {
	HMACSecret []byte
	// MaxSkew rejects tokens with iat/exp outside this skew (default 2m).
	MaxSkew time.Duration
}

func (p JWTProvider) Authenticate(r *http.Request) (Principal, error) {
	if len(p.HMACSecret) == 0 {
		return Principal{}, fmt.Errorf("%w: jwt hmac secret not configured", ErrUnauthenticated)
	}
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		return Principal{}, ErrUnauthenticated
	}
	token := strings.TrimSpace(authz[7:])
	claims, err := parseHS256JWT(token, p.HMACSecret, p.MaxSkew)
	if err != nil {
		return Principal{}, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	tenant := firstClaim(claims, "tenant_id", "tid")
	actor := firstClaim(claims, "sub", "actor_id", "aid")
	if tenant == "" || actor == "" {
		return Principal{}, fmt.Errorf("%w: missing tenant/actor claims", ErrUnauthenticated)
	}
	return Principal{
		TenantID:    tenant,
		ActorID:     actor,
		Roles:       stringSliceClaim(claims, "roles"),
		Permissions: stringSliceClaim(claims, "permissions"),
	}, nil
}

func firstClaim(claims map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := claims[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func stringSliceClaim(claims map[string]any, key string) []string {
	v, ok := claims[key]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		return t
	case string:
		return splitCSV(t)
	default:
		return nil
	}
}

func parseHS256JWT(token string, secret []byte, skew time.Duration) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid jwt format")
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("header decode: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, err
	}
	if !strings.EqualFold(header.Alg, "HS256") {
		return nil, fmt.Errorf("unsupported alg %q", header.Alg)
	}
	signing := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(signing))
	want := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("sig decode: %w", err)
	}
	if !hmac.Equal(want, got) {
		return nil, fmt.Errorf("bad signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("payload decode: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	if skew <= 0 {
		skew = 2 * time.Minute
	}
	now := time.Now().UTC()
	if exp, ok := numericClaim(claims, "exp"); ok {
		if now.After(time.Unix(exp, 0).Add(skew)) {
			return nil, fmt.Errorf("token expired")
		}
	}
	if nbf, ok := numericClaim(claims, "nbf"); ok {
		if now.Before(time.Unix(nbf, 0).Add(-skew)) {
			return nil, fmt.Errorf("token not yet valid")
		}
	}
	return claims, nil
}

func numericClaim(claims map[string]any, key string) (int64, bool) {
	v, ok := claims[key]
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case json.Number:
		i, err := t.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}

// IssueHS256JWT is a test helper to mint tokens for JWTProvider.
func IssueHS256JWT(secret []byte, claims map[string]any) (string, error) {
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	h := base64.RawURLEncoding.EncodeToString(header)
	p := base64.RawURLEncoding.EncodeToString(payload)
	signing := h + "." + p
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(signing))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signing + "." + sig, nil
}
