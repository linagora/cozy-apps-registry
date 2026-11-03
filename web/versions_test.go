package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cozy/cozy-apps-registry/auth"
	"github.com/cozy/cozy-apps-registry/base"
	"github.com/cozy/cozy-apps-registry/registry"
	"github.com/cozy/cozy-apps-registry/space"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPublisher creates an app owned by a new editor in the all-apps space, and
// returns a token that only allows to publish this app.
func newPublisher(t *testing.T, slug string) []byte {
	t.Helper()
	if base.SessionSecret == nil {
		base.SessionSecret = auth.GenerateMasterSecret()
		t.Cleanup(func() { base.SessionSecret = nil })
	}
	editor, err := auth.Editors.CreateEditorWithoutPublicKey(strings.ReplaceAll(slug, "-", ""), true)
	require.NoError(t, err)
	t.Cleanup(func() { _ = auth.Editors.DeleteEditor(editor) })

	s, _ := space.GetSpace(allAppsSpace)
	opts := &registry.AppOptions{Editor: editor.Name(), Slug: slug, Type: "webapp"}
	_, err = registry.CreateApp(s, opts, editor)
	require.NoError(t, err)

	token, err := editor.GenerateEditorToken(base.SessionSecret, 0, slug)
	require.NoError(t, err)
	return token
}

// postVersion publishes a version of an app of the all-apps space, and returns
// the status code and the body of the response.
func postVersion(t *testing.T, token []byte, slug, version, tarballURL, shasum string) (int, string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"version": version,
		"url":     tarballURL,
		"sha256":  shasum,
	})
	require.NoError(t, err)
	target := fmt.Sprintf("%s/%s/registry/%s", server.URL, allAppsSpace, slug)
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAuthorization, authTokenScheme+base64.StdEncoding.EncodeToString(token))
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	resBody, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(resBody)
}

func TestCreateVersionRefusesLocalFile(t *testing.T) {
	token := newPublisher(t, "file-url-app")

	content := []byte("a local file of the server")
	secret := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(secret, content, 0600))
	sum := sha256.Sum256(content)

	for _, fileURL := range []string{"file://" + secret, "FILE://" + secret, "file:" + secret} {
		status, body := postVersion(t, token, "file-url-app", "1.0.0", fileURL, strings.Repeat("0", 64))
		assert.Equal(t, http.StatusBadRequest, status, fileURL)
		assert.Contains(t, body, "erroneous: url", fileURL)
		// The checksum error would give away the hash of the file
		assert.NotContains(t, body, hex.EncodeToString(sum[:]), fileURL)
	}
}
