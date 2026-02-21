package service_test

import (
	"errors"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/unicode/norm"

	"github.com/enunezf/sentinel/internal/service"
)

// TestPasswordPolicy_TooShort verifies the 10-character minimum.
func TestPasswordPolicy_TooShort(t *testing.T) {
	// 9 chars: must be rejected.
	err := service.ValidatePasswordPolicy("Sh0rt!Sym")
	assert.Error(t, err, "9-char password must be rejected")
	assert.True(t, errors.Is(err, service.ErrPasswordPolicy))

	// 10 chars: must be accepted (meets all other requirements too).
	err = service.ValidatePasswordPolicy("Valid1!Sym0")
	assert.NoError(t, err, "10-char password meeting all rules must be accepted")
}

// TestPasswordPolicy_NoUppercase verifies that missing uppercase is rejected.
func TestPasswordPolicy_NoUppercase(t *testing.T) {
	// All lowercase, has digit and symbol, 10+ chars.
	err := service.ValidatePasswordPolicy("lowercase1!abc")
	assert.Error(t, err, "password without uppercase must be rejected")
	assert.True(t, errors.Is(err, service.ErrPasswordPolicy))
}

// TestPasswordPolicy_NoNumber verifies that missing digit is rejected.
func TestPasswordPolicy_NoNumber(t *testing.T) {
	// Has uppercase and symbol but no digit.
	err := service.ValidatePasswordPolicy("NoNumber!Abcdef")
	assert.Error(t, err, "password without number must be rejected")
	assert.True(t, errors.Is(err, service.ErrPasswordPolicy))
}

// TestPasswordPolicy_NoSymbol verifies that missing symbol is rejected.
func TestPasswordPolicy_NoSymbol(t *testing.T) {
	// Has uppercase, digit, 10+ chars but no symbol.
	err := service.ValidatePasswordPolicy("NoSymbol12345A")
	assert.Error(t, err, "password without symbol must be rejected")
	assert.True(t, errors.Is(err, service.ErrPasswordPolicy))
}

// TestPasswordPolicy_ValidPassword verifies a fully compliant password is accepted.
func TestPasswordPolicy_ValidPassword(t *testing.T) {
	validPasswords := []string{
		"S3cur3P@ss!",
		"MyPassword1$",
		"Abc123!@#xyz",
		"T3mpP@ssw0rd!",
	}
	for _, pwd := range validPasswords {
		err := service.ValidatePasswordPolicy(pwd)
		assert.NoError(t, err, "valid password %q must be accepted", pwd)
	}
}

// TestPasswordPolicy_UnicodeNFC verifies a password with an accented character in NFC
// passes policy validation when it meets all requirements.
func TestPasswordPolicy_UnicodeNFC(t *testing.T) {
	// "Ñoño" in NFC with digits and symbol: 12 code points, has uppercase, digit, symbol.
	pwd := norm.NFC.String("Contraseña1!")
	err := service.ValidatePasswordPolicy(pwd)
	assert.NoError(t, err, "NFC password with accent must be accepted")
}

// TestPasswordPolicy_UnicodeNFD verifies that a password in NFD form, once normalized
// to NFC, produces the same bcrypt hash as the NFC form.
func TestPasswordPolicy_UnicodeNFD(t *testing.T) {
	// "Contraseña1!" can be represented as NFC or NFD.
	// NFC: precomposed ñ (U+00F1)
	// NFD: n + combining tilde (U+006E + U+0303)
	nfcPwd := norm.NFC.String("Contraseña1!")
	nfdPwd := norm.NFD.String("Contraseña1!")

	// They must differ in byte representation.
	// NFD has more bytes due to decomposed form.
	if nfcPwd == nfdPwd {
		t.Skip("platform produces identical NFC/NFD for this string")
	}

	// After normalizing NFD to NFC, both should produce the same hash.
	nfdNormalized := norm.NFC.String(nfdPwd)
	assert.Equal(t, nfcPwd, nfdNormalized, "NFC(NFD(pwd)) must equal NFC(pwd)")

	// Bcrypt hash of NFC must match comparison with NFC-normalized NFD input.
	hash, err := bcrypt.GenerateFromPassword([]byte(nfcPwd), 4)
	require.NoError(t, err)

	err = bcrypt.CompareHashAndPassword(hash, []byte(nfdNormalized))
	assert.NoError(t, err, "NFC-normalized NFD password must match NFC hash")
}

// TestPasswordPolicy_UnicodeMinLength verifies that length is measured in Unicode
// code points, not bytes.
func TestPasswordPolicy_UnicodeMinLength(t *testing.T) {
	// Each of these chars is multi-byte in UTF-8 but is 1 code point.
	// Build exactly 9 code-point password: should fail.
	// "Ñ" = U+00D1 (2 bytes in UTF-8), "1" digit, "!" symbol.
	// 9 code points: "ÑABCDE1!x" = Ñ(1)+A(1)+B(1)+C(1)+D(1)+E(1)+1(1)+!(1)+x(1) = 9
	nineCPPwd := "ÑABCDE1!x"
	assert.Equal(t, 9, utf8.RuneCountInString(nineCPPwd), "must be exactly 9 code points")
	err := service.ValidatePasswordPolicy(nineCPPwd)
	assert.Error(t, err, "9 code-point password must be rejected")

	// 10 code points: "ÑABCDE1!xy"
	tenCPPwd := "ÑABCDE1!xy"
	assert.Equal(t, 10, utf8.RuneCountInString(tenCPPwd), "must be exactly 10 code points")
	err = service.ValidatePasswordPolicy(tenCPPwd)
	assert.NoError(t, err, "10 code-point password meeting all rules must be accepted")
}

// TestPasswordPolicy_Emoji verifies that an emoji counts as a symbol.
func TestPasswordPolicy_Emoji(t *testing.T) {
	// "Password1" + emoji: 10 chars, has uppercase, digit, emoji (symbol).
	// "Password1🔑" = 10 code points.
	emojiPwd := "Password1🔑"
	assert.Equal(t, 10, utf8.RuneCountInString(emojiPwd))
	err := service.ValidatePasswordPolicy(emojiPwd)
	assert.NoError(t, err, "emoji should count as a symbol and allow the password")
}
