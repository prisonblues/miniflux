// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cli

// Note: resolveUser requires a real *storage.Storage backed by a database,
// so full integration tests for user resolution belong in the integration test suite.
// The function is tested indirectly through the -mcp flag's startup path.
//
// Unit tests here verify the MCP flag constant and help text are defined correctly.

import (
	"testing"
)

func TestMCPHelpText(t *testing.T) {
	if flagMCPHelp == "" {
		t.Error("MCP help text should not be empty")
	}
}
