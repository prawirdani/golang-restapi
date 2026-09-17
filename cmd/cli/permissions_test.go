package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// permissionLiteral matches `<ident> rbac.Permission = "value"` declarations.
var permissionLiteral = regexp.MustCompile(`rbac\.Permission\s*=\s*"([^"]+)"`)

// TestGatherPermissions_CoversEveryDefinedPermission guards the hand-maintained
// registrar list in gatherPermissions: every rbac.Permission constant declared
// in internal/ must appear in the dumped set. Adding a domain (or a permission)
// without replaying its registrar then fails here instead of silently drifting.
func TestGatherPermissions_CoversEveryDefinedPermission(t *testing.T) {
	root := filepath.Join("..", "..", "internal")

	var defined []string
	seen := map[string]struct{}{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range permissionLiteral.FindAllStringSubmatch(string(b), -1) {
			if _, ok := seen[m[1]]; ok {
				continue
			}
			seen[m[1]] = struct{}{}
			defined = append(defined, m[1])
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, defined, "expected to find permission constants under internal/")

	gathered := gatherPermissions()
	for _, perm := range defined {
		assert.Contains(t, gathered, perm, "declared permission missing from the dumped set")
	}
}
