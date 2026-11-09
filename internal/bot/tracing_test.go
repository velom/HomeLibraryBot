package bot

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelegramClientRedactsTokenFromErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL + "/bot123:SECRET/sendMessage"
	srv.Close()

	req, err := http.NewRequest(http.MethodPost, url, nil)
	require.NoError(t, err)

	_, err = newTelegramClient().Do(req)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRET")
	assert.Contains(t, err.Error(), "telegram:sendMessage")
}

func TestCommandOf(t *testing.T) {
	assert.Equal(t, "/read", commandOf("/read"))
	assert.Equal(t, "/ask", commandOf("/ask@library_bot how many books?"))
	assert.Equal(t, "", commandOf("hello"))
	assert.Equal(t, "", commandOf(""))
}
