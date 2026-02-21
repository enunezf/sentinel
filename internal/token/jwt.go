package token

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/enunezf/sentinel/internal/domain"
)

// KID is the key identifier embedded in the JWT header and JWKS.
const KID = "2026-02-key-01"

// Manager holds RSA key material and handles JWT operations.
type Manager struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
}

// NewManagerFromKey creates a Manager directly from an RSA private key.
// Useful for tests that generate keys in memory without PEM files.
func NewManagerFromKey(privateKey *rsa.PrivateKey) *Manager {
	return &Manager{privateKey: privateKey, publicKey: &privateKey.PublicKey}
}

// NewManager loads RSA keys from PEM files and returns a Manager.
func NewManager(privateKeyPath, publicKeyPath string) (*Manager, error) {
	privPEM, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("token: read private key: %w", err)
	}
	block, _ := pem.Decode(privPEM)
	if block == nil {
		return nil, fmt.Errorf("token: failed to decode private key PEM")
	}
	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8.
		key, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("token: parse private key: %w", err)
		}
		var ok bool
		privKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("token: private key is not RSA")
		}
	}

	pubPEM, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("token: read public key: %w", err)
	}
	pubBlock, _ := pem.Decode(pubPEM)
	if pubBlock == nil {
		return nil, fmt.Errorf("token: failed to decode public key PEM")
	}
	pubInterface, err := x509.ParsePKIXPublicKey(pubBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("token: parse public key: %w", err)
	}
	pubKey, ok := pubInterface.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("token: public key is not RSA")
	}

	return &Manager{privateKey: privKey, publicKey: pubKey}, nil
}

// sentinelClaims extends jwt.RegisteredClaims with Sentinel-specific fields.
type sentinelClaims struct {
	Username string   `json:"username"`
	Email    string   `json:"email"`
	App      string   `json:"app"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

// GenerateAccessToken creates a signed RS256 JWT for the given user.
func (m *Manager) GenerateAccessToken(user *domain.User, appSlug string, roles []string, ttl time.Duration) (string, error) {
	now := time.Now()
	jti := uuid.New().String()

	claims := sentinelClaims{
		Username: user.Username,
		Email:    user.Email,
		App:      appSlug,
		Roles:    roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        jti,
		},
	}

	t := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	t.Header["kid"] = KID

	signed, err := t.SignedString(m.privateKey)
	if err != nil {
		return "", fmt.Errorf("token: sign JWT: %w", err)
	}
	return signed, nil
}

// ValidateToken parses and validates a JWT string, returning the domain Claims.
func (m *Manager) ValidateToken(tokenStr string) (*domain.Claims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &sentinelClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("token: unexpected signing method: %v", t.Header["alg"])
		}
		return m.publicKey, nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		return nil, err
	}

	sc, ok := t.Claims.(*sentinelClaims)
	if !ok || !t.Valid {
		return nil, fmt.Errorf("token: invalid claims")
	}

	exp := int64(0)
	if sc.ExpiresAt != nil {
		exp = sc.ExpiresAt.Unix()
	}
	iat := int64(0)
	if sc.IssuedAt != nil {
		iat = sc.IssuedAt.Unix()
	}

	return &domain.Claims{
		Sub:      sc.Subject,
		Username: sc.Username,
		Email:    sc.Email,
		App:      sc.App,
		Roles:    sc.Roles,
		Iat:      iat,
		Exp:      exp,
		Jti:      sc.ID,
	}, nil
}

// JWKSKey represents a single key in the JWKS document (RFC 7517).
type JWKSKey struct {
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKSResponse is the full JWKS document.
type JWKSResponse struct {
	Keys []JWKSKey `json:"keys"`
}

// GenerateJWKS returns the JWKS document for the loaded public key.
func (m *Manager) GenerateJWKS() JWKSResponse {
	nBytes := m.publicKey.N.Bytes()
	n := base64.RawURLEncoding.EncodeToString(nBytes)

	eVal := big.NewInt(int64(m.publicKey.E))
	e := base64.RawURLEncoding.EncodeToString(eVal.Bytes())

	return JWKSResponse{
		Keys: []JWKSKey{
			{
				Kty: "RSA",
				Alg: "RS256",
				Use: "sig",
				Kid: KID,
				N:   n,
				E:   e,
			},
		},
	}
}

// SignPayload signs arbitrary bytes with RSA-SHA256 and returns base64url encoding.
// Used for signing the permissions map.
func (m *Manager) SignPayload(payload []byte) (string, error) {
	digest := sha256.Sum256(payload)
	sig, err := rsa.SignPKCS1v15(rand.Reader, m.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("token: sign payload: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(sig), nil
}

// PublicKey returns the RSA public key for external use.
func (m *Manager) PublicKey() *rsa.PublicKey {
	return m.publicKey
}
