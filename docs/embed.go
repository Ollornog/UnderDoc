// SPDX-License-Identifier: MIT

// Package docs makes the logo files available to the server, so they exist only once in the repository.
package docs

import _ "embed"

// Logo is the logo. It adapts to the color scheme by itself.
//
//go:embed logo.svg
var Logo []byte

// Favicon is the icon for the browser tab.
//
//go:embed favicon.ico
var Favicon []byte
