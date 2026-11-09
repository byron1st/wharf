package buildinfo_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/buildinfo"
)

func TestFormat(t *testing.T) {
	tests := map[string]string{
		"v0.1.0":                               "0.1.0",
		"v1.2.3+dirty":                         "1.2.3+dirty",
		"v0.1.1-0.20261109060017-7897bbd61e4c": "0.1.1-0.20261109060017-7897bbd61e4c",
		"(devel)":                              "(devel)",
		"":                                     "(devel)",
	}

	for in, want := range tests {
		require.Equal(t, want, buildinfo.Format(in), "version=%q", in)
	}
}

func TestVersionIsStamped(t *testing.T) {
	require.NotEmpty(t, buildinfo.Version())
}
