// Package token gestiona la generación, validación y publicación de claves JWT RS256.
// Toda la criptografía de tokens de acceso pasa por este paquete.
// Las claves RSA se cargan desde archivos PEM en el arranque del servidor;
// nunca se incluyen en la imagen Docker (se montan como volumen).
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

// KID es el identificador de clave (Key ID) que se embebe en el header del JWT
// y en el documento JWKS. Los backends consumidores usan el KID para seleccionar
// la clave pública correcta cuando rotan las claves.
const KID = "2026-02-key-01"

// Manager encapsula el par de claves RSA y expone todas las operaciones criptográficas
// de Sentinel: generación de JWT, validación de JWT, publicación de JWKS y firma
// de payloads arbitrarios (mapa de permisos).
type Manager struct {
	privateKey *rsa.PrivateKey // clave privada RSA para firmar JWT y payloads
	publicKey  *rsa.PublicKey  // clave pública RSA derivada de la privada, para validación
}

// NewManagerFromKey crea un Manager directamente a partir de una clave RSA privada
// en memoria. Se usa en pruebas unitarias que generan claves RSA al vuelo para
// evitar depender de archivos PEM en el sistema de archivos.
func NewManagerFromKey(privateKey *rsa.PrivateKey) *Manager {
	return &Manager{privateKey: privateKey, publicKey: &privateKey.PublicKey}
}

// NewManager carga las claves RSA desde archivos PEM y devuelve un Manager listo para usar.
// Acepta claves en formato PKCS#1 (RSA PRIVATE KEY) y PKCS#8 (PRIVATE KEY).
//
// Parámetros:
//   - privateKeyPath: ruta al archivo PEM de la clave privada (ej: "keys/private.pem").
//   - publicKeyPath: ruta al archivo PEM de la clave pública (ej: "keys/public.pem").
//
// Retorna error si los archivos no existen, no son PEM válidos o no contienen claves RSA.
func NewManager(privateKeyPath, publicKeyPath string) (*Manager, error) {
	privPEM, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("token: read private key: %w", err)
	}
	block, _ := pem.Decode(privPEM)
	if block == nil {
		return nil, fmt.Errorf("token: failed to decode private key PEM")
	}

	// Intentar PKCS#1 primero; si falla, intentar PKCS#8.
	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
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

// sentinelClaims extiende jwt.RegisteredClaims con los campos específicos de Sentinel
// que se incluyen en el payload del JWT. Los campos exportados son los que aparecen
// en el JSON del token.
type sentinelClaims struct {
	Username string   `json:"username"` // nombre del usuario autenticado
	Email    string   `json:"email"`    // correo electrónico del usuario
	App      string   `json:"app"`      // slug de la aplicación (ej: "system", "erp")
	Roles    []string `json:"roles"`    // nombres de los roles activos del usuario en la app
	jwt.RegisteredClaims
	// RegisteredClaims incluye: sub (UUID del usuario), iat (issued at), exp (expiry), jti (JWT ID).
}

// GenerateAccessToken crea y firma un JWT RS256 para el usuario en la aplicación indicada.
// El token incluye el KID en el header para que los backends consumidores puedan
// seleccionar la clave correcta del JWKS sin necesidad de inspeccionar el payload.
//
// Parámetros:
//   - user: entidad del usuario autenticado (ID, username, email).
//   - appSlug: slug de la aplicación para la que se genera el token.
//   - roles: nombres de los roles activos del usuario en esa aplicación.
//   - ttl: tiempo de vida del token (ej: 15 minutos para access tokens).
//
// Retorna el JWT firmado como string o un error si la firma falla.
func (m *Manager) GenerateAccessToken(user *domain.User, appSlug string, roles []string, ttl time.Duration) (string, error) {
	now := time.Now()
	// JTI único por token: se usa como clave de caché en Redis (AuthzService).
	jti := uuid.New().String()

	claims := sentinelClaims{
		Username: user.Username,
		Email:    user.Email,
		App:      appSlug,
		Roles:    roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(), // sub = UUID del usuario
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        jti,
		},
	}

	t := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	// Agregar KID al header para que los backends consumidores puedan identificar
	// la clave pública correcta en el JWKS sin necesidad de probar todas.
	t.Header["kid"] = KID

	signed, err := t.SignedString(m.privateKey)
	if err != nil {
		return "", fmt.Errorf("token: sign JWT: %w", err)
	}
	return signed, nil
}

// ValidateToken parsea y valida un JWT RS256, devolviendo los claims de dominio.
// Verificaciones que realiza:
//   - El algoritmo de firma es RS256 (rechaza HS256 y otros).
//   - La firma es válida usando la clave pública cargada.
//   - El token no ha expirado (exp > now).
//
// Retorna un error de la librería jwt si el token está malformado, tiene firma
// inválida o ha expirado. En caso de éxito, devuelve los claims como domain.Claims.
func (m *Manager) ValidateToken(tokenStr string) (*domain.Claims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &sentinelClaims{}, func(t *jwt.Token) (interface{}, error) {
		// Rechazar cualquier algoritmo que no sea RSA (previene ataques de algoritmo "none").
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

	// Extraer los timestamps Unix para incluirlos en domain.Claims.
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

// JWKSKey representa una única clave en el documento JWKS (RFC 7517).
// Los backends consumidores usan este documento para obtener la clave pública
// y verificar tokens localmente sin necesidad de llamar a Sentinel en cada request.
type JWKSKey struct {
	Kty string `json:"kty"` // tipo de clave: siempre "RSA"
	Alg string `json:"alg"` // algoritmo: siempre "RS256"
	Use string `json:"use"` // uso: siempre "sig" (firma)
	Kid string `json:"kid"` // identificador de clave (KID)
	N   string `json:"n"`   // módulo RSA en base64url sin padding
	E   string `json:"e"`   // exponente público RSA en base64url sin padding
}

// JWKSResponse es el documento JWKS completo servido en /.well-known/jwks.json.
// Contiene una sola clave activa; al rotar claves se publicarían ambas
// (antigua y nueva) durante un período de solapamiento.
type JWKSResponse struct {
	Keys []JWKSKey `json:"keys"`
}

// GenerateJWKS construye el documento JWKS con la clave pública RSA cargada.
// El módulo (N) y el exponente (E) se codifican en base64url sin padding,
// según lo establece la RFC 7517 para claves RSA.
// Este método no tiene efectos secundarios; puede llamarse múltiples veces.
func (m *Manager) GenerateJWKS() JWKSResponse {
	// Codificar el módulo RSA en base64url (big-endian, sin padding).
	nBytes := m.publicKey.N.Bytes()
	n := base64.RawURLEncoding.EncodeToString(nBytes)

	// Codificar el exponente público como big-endian bytes en base64url.
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

// SignPayload firma bytes arbitrarios con RSA-SHA256 (PKCS#1 v1.5) y devuelve
// la firma en base64url sin padding.
// Se usa para firmar el mapa canónico de permisos de AuthzService, permitiendo
// que los backends consumidores verifiquen su integridad con la clave pública
// del JWKS.
//
// Proceso:
//  1. Calcula SHA-256 del payload.
//  2. Firma el digest con rsa.SignPKCS1v15 usando la clave privada.
//  3. Codifica la firma en base64url sin padding.
//
// Parámetros:
//   - payload: bytes a firmar (normalmente el JSON canónico del mapa de permisos).
//
// Retorna la firma como string base64url o un error si la operación criptográfica falla.
func (m *Manager) SignPayload(payload []byte) (string, error) {
	digest := sha256.Sum256(payload)
	sig, err := rsa.SignPKCS1v15(rand.Reader, m.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("token: sign payload: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(sig), nil
}

// PublicKey devuelve la clave pública RSA cargada en el Manager.
// Se usa en pruebas para verificar firmas generadas por SignPayload o
// para inspeccionar los parámetros de la clave sin pasar por el JWKS.
func (m *Manager) PublicKey() *rsa.PublicKey {
	return m.publicKey
}
