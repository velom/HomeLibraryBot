package bot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testBotToken = "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"

// generateTestInitData builds a valid Telegram initData string signed with the given token.
// This mirrors Telegram's signing algorithm: HMAC-SHA256 of sorted key=value pairs,
// using a secret derived from HMAC-SHA256("WebAppData", botToken).
func generateTestInitData(t *testing.T, token string, userID int64, authDate time.Time) string {
	t.Helper()

	userData, err := json.Marshal(map[string]interface{}{
		"id":         userID,
		"first_name": "Test",
		"username":   "testuser",
	})
	require.NoError(t, err)

	params := url.Values{}
	params.Set("user", string(userData))
	params.Set("auth_date", fmt.Sprintf("%d", authDate.Unix()))
	params.Set("query_id", "test-query-id")

	// Build data-check-string (sorted key=value pairs joined by \n)
	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var dataCheckString strings.Builder
	for i, k := range keys {
		if i > 0 {
			dataCheckString.WriteByte('\n')
		}
		dataCheckString.WriteString(k)
		dataCheckString.WriteByte('=')
		dataCheckString.WriteString(params.Get(k))
	}

	// Compute HMAC-SHA256 hash
	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(token))
	secret := secretKey.Sum(nil)

	h := hmac.New(sha256.New, secret)
	h.Write([]byte(dataCheckString.String()))
	hash := hex.EncodeToString(h.Sum(nil))

	params.Set("hash", hash)
	return params.Encode()
}

func TestValidateInitData_Valid(t *testing.T) {
	allowed := map[int64]bool{42: true}
	initData := generateTestInitData(t, testBotToken, 42, time.Now())

	userID, err := validateTelegramInitData(initData, testBotToken, allowed)
	require.NoError(t, err)
	assert.Equal(t, int64(42), userID)
}

func TestValidateInitData_EmptyString(t *testing.T) {
	allowed := map[int64]bool{42: true}
	_, err := validateTelegramInitData("", testBotToken, allowed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing initData")
}

func TestValidateInitData_TamperedHash(t *testing.T) {
	allowed := map[int64]bool{42: true}
	initData := generateTestInitData(t, testBotToken, 42, time.Now())
	// Replace last char of hash to tamper it
	initData = strings.Replace(initData, "hash=", "hash=00", 1)

	_, err := validateTelegramInitData(initData, testBotToken, allowed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hash")
}

func TestValidateInitData_ExpiredAuthDate(t *testing.T) {
	allowed := map[int64]bool{42: true}
	initData := generateTestInitData(t, testBotToken, 42, time.Now().Add(-25*time.Hour))

	_, err := validateTelegramInitData(initData, testBotToken, allowed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too old")
}

func TestValidateInitData_UserNotAllowed(t *testing.T) {
	allowed := map[int64]bool{99: true} // userID 42 is not allowed
	initData := generateTestInitData(t, testBotToken, 42, time.Now())

	_, err := validateTelegramInitData(initData, testBotToken, allowed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestValidateInitData_WrongToken(t *testing.T) {
	allowed := map[int64]bool{42: true}
	initData := generateTestInitData(t, testBotToken, 42, time.Now())

	_, err := validateTelegramInitData(initData, "wrong-token", allowed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hash")
}

func TestValidateInitData_MissingUserField(t *testing.T) {
	// Build initData without a "user" parameter
	params := url.Values{}
	params.Set("auth_date", fmt.Sprintf("%d", time.Now().Unix()))
	params.Set("query_id", "test-query-id")

	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var dcs strings.Builder
	for i, k := range keys {
		if i > 0 {
			dcs.WriteByte('\n')
		}
		dcs.WriteString(k + "=" + params.Get(k))
	}
	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(testBotToken))
	h := hmac.New(sha256.New, secretKey.Sum(nil))
	h.Write([]byte(dcs.String()))
	params.Set("hash", hex.EncodeToString(h.Sum(nil)))

	allowed := map[int64]bool{42: true}
	_, err := validateTelegramInitData(params.Encode(), testBotToken, allowed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing user")
}
