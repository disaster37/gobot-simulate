package server

import (
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/disaster37/gobot-simulate/webui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sha256Hash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

func TestContentSecurityPolicyHashesInlineScripts(t *testing.T) {
	importMap := `{"imports":{"#entry":"/_nuxt/entry.js"}}`
	bootstrap := `console.log("bootstrap")`
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<!DOCTYPE html><html><head>` +
			`<script type="importmap">` + importMap + `</script>` +
			`<script src="/_nuxt/entry.js"></script>` +
			`<script>` + bootstrap + `</script>` +
			`<script type="module">` + bootstrap + `</script>` +
			`</head></html>`)},
	}

	csp := buildContentSecurityPolicy(assets)
	assert.Contains(t, csp, sha256Hash(importMap))
	assert.Contains(t, csp, sha256Hash(bootstrap))
	assert.NotContains(t, csp, "script-src 'self' 'unsafe-inline'")
	assert.Equal(t, 2, strings.Count(csp, "sha256-"), "duplicate inline content must not duplicate hashes")
	assert.Contains(t, csp, "style-src 'self' 'unsafe-inline'")
}

func TestContentSecurityPolicyWithoutInlineScripts(t *testing.T) {
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<html><head><script src="/app.js"></script></head></html>`)},
	}
	csp := buildContentSecurityPolicy(assets)
	assert.Contains(t, csp, "script-src 'self';")
	assert.NotContains(t, csp, "sha256-")
}

func TestContentSecurityPolicyHandlesMissingIndex(t *testing.T) {
	csp := buildContentSecurityPolicy(fstest.MapFS{})
	assert.Contains(t, csp, "script-src 'self';")
}

func TestEmbeddedImportMapHash(t *testing.T) {
	index, err := fs.ReadFile(webui.DistFS(), "index.html")
	require.NoError(t, err)

	matches := regexp.MustCompile(`(?is)<script\b[^>]*type="importmap"[^>]*>(.*?)</script\s*>`).FindSubmatch(index)
	require.NotNil(t, matches, "embedded index.html must contain an inline import map")
	expected := sha256Hash(string(matches[1]))

	csp := buildContentSecurityPolicy(webui.DistFS())
	assert.Contains(t, csp, expected)
	assert.NotContains(t, csp, "script-src 'self' 'unsafe-inline'")
}
