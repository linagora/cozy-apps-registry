package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/cozy/cozy-apps-registry/base"
	"github.com/cozy/cozy-apps-registry/space"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const legacyContent = "the content of an asset stored in the app container"

// createLegacyVersion publishes a version whose asset still lives in the app
// container, at <slug>/<version>/<filename>, like the ones published before
// the global asset store.
func createLegacyVersion(t *testing.T, s *space.Space, slug, version, filename string) *Version {
	t.Helper()
	ver := &Version{
		ID:        getVersionID(slug, version),
		Slug:      slug,
		Version:   version,
		Type:      "webapp",
		Manifest:  json.RawMessage(`{}`),
		CreatedAt: time.Now(),
		URL:       "https://registry.example.org/registry/" + slug + "/" + version + "/tarball/" + filename,
	}
	rev, err := s.VersDB().Put(context.Background(), ver.ID, ver)
	require.NoError(t, err)
	ver.Rev = rev

	fp := slug + "/" + version + "/" + filename
	require.NoError(t, base.Storage.Create(s.GetPrefix(), fp, "application/gzip", bytes.NewReader([]byte(legacyContent))))
	return ver
}

func TestFindVersionAttachmentStaysInTheVersionFolder(t *testing.T) {
	s, _ := space.GetSpace(testSpaceName)
	const filename = "app.tar.gz"
	ver := createLegacyVersion(t, s, "traversal-reader", "1.0.0", filename)
	createLegacyVersion(t, s, "traversal-victim", "1.0.0", filename)

	for _, name := range []string{
		"../../traversal-victim/1.0.0/" + filename,
		"screenshots/../../../traversal-victim/1.0.0/" + filename,
		"/traversal-victim/1.0.0/" + filename,
		"..",
		".",
		"screenshots/..",
		"",
	} {
		_, err := FindVersionAttachment(s, ver, name)
		assert.ErrorIs(t, err, base.ErrFileNotFound, name)
	}

	// A name with a folder of its own is still read. The version has no ID so
	// that the read does not start moving the asset.
	const screenshot = "screenshots/one.png"
	require.NoError(t, base.Storage.Create(s.GetPrefix(), "traversal-reader/1.0.0/"+screenshot, "image/png", bytes.NewReader([]byte(legacyContent))))
	reader := *ver
	reader.ID = ""
	att, err := FindVersionAttachment(s, &reader, screenshot)
	require.NoError(t, err)
	content, err := io.ReadAll(att.Content)
	require.NoError(t, err)
	assert.Equal(t, legacyContent, string(content))
}
