// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/samba"
	"github.com/Ollornog/UnderDoc/internal/share"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// readSambaConfig reads the setup of Samba (ADR-30, docs/samba.md): the companion of each tenant, the share directory, the
// name of the server and the range of UIDs. Without UNDERDOC_SAMBA nothing is shared.
//
//	UNDERDOC_SAMBA=main=/run/underdoc-samba/helper.sock
//	UNDERDOC_SHARE_DIR=/srv/underdoc-share
//	UNDERDOC_SAMBA_SERVER=files
//	UNDERDOC_SAMBA_UIDS=20000-29999
func readSambaConfig() (share.Config, bool, error) {
	spec := strings.TrimSpace(os.Getenv("UNDERDOC_SAMBA"))
	if spec == "" {
		return share.Config{}, false, nil
	}
	cfg := share.Config{Dir: os.Getenv("UNDERDOC_SHARE_DIR"), Helpers: map[string]share.Helper{}, Server: env("UNDERDOC_SAMBA_SERVER", "")}
	for _, part := range strings.Split(spec, ",") {
		t, socket, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || !tenant.Valid(t) || !filepath.IsAbs(socket) {
			return cfg, false, fmt.Errorf("UNDERDOC_SAMBA: tenant=/path/of/the/socket, separated by commas")
		}
		cfg.Helpers[t] = samba.Client{Socket: socket, Timeout: time.Minute}
	}
	if cfg.Dir == "" || !filepath.IsAbs(cfg.Dir) {
		return cfg, false, fmt.Errorf("UNDERDOC_SHARE_DIR: the absolute directory of the working copy, the same in the companion")
	}
	if cfg.Server == "" {
		return cfg, false, fmt.Errorf("UNDERDOC_SAMBA_SERVER: the name of the server as people type it, such as files")
	}
	lo, hi, ok := strings.Cut(env("UNDERDOC_SAMBA_UIDS", "20000-29999"), "-")
	var err error
	if cfg.UIDMin, err = strconv.Atoi(lo); err != nil || !ok {
		return cfg, false, fmt.Errorf("UNDERDOC_SAMBA_UIDS: a range like 20000-29999")
	}
	if cfg.UIDMax, err = strconv.Atoi(hi); err != nil || cfg.UIDMax < cfg.UIDMin || cfg.UIDMin < 1000 || cfg.UIDMax > 65534 {
		return cfg, false, fmt.Errorf("UNDERDOC_SAMBA_UIDS: a range like 20000-29999, within 1000 to 65534")
	}
	return cfg, true, nil
}
