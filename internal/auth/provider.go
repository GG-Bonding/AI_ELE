package auth

import (
	"fmt"
	"strings"
)

// Mode selects PrincipalProvider implementation.
type Mode string

const (
	ModeNone       Mode = "none"
	ModeDevHeaders Mode = "dev_headers"
	ModeJWT        Mode = "jwt"
)

// Config for process auth wiring (V3.4.1).
type Config struct {
	Mode          Mode   `yaml:"mode" json:"mode"`
	JWTHMACSecret string `yaml:"jwt_hmac_secret" json:"jwt_hmac_secret"`
}

// NewProvider builds a PrincipalProvider from config.
func NewProvider(cfg Config) (PrincipalProvider, error) {
	mode := Mode(strings.ToLower(strings.TrimSpace(string(cfg.Mode))))
	if mode == "" {
		mode = ModeNone
	}
	switch mode {
	case ModeNone:
		return NoneProvider{}, nil
	case ModeDevHeaders:
		return DevHeaderProvider{}, nil
	case ModeJWT:
		secret := strings.TrimSpace(cfg.JWTHMACSecret)
		if secret == "" {
			return nil, fmt.Errorf("auth: jwt mode requires jwt_hmac_secret")
		}
		return JWTProvider{HMACSecret: []byte(secret)}, nil
	default:
		return nil, fmt.Errorf("auth: unsupported mode %q (want none|dev_headers|jwt)", cfg.Mode)
	}
}
