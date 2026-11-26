// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
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
//	UNDERDOC_SAMBA_DOMAIN=main               tenants whose companion is a member of a domain (T-42)
//	UNDERDOC_SAMBA_DOMAIN_NAMES=login        what names the domain account: login or remote-id
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
	if spec := strings.TrimSpace(os.Getenv("UNDERDOC_SAMBA_DOMAIN")); spec != "" {
		cfg.Domain = map[string]bool{}
		for _, t := range strings.Split(spec, ",") {
			t = strings.TrimSpace(t)
			if cfg.Helpers[t] == nil {
				return cfg, false, fmt.Errorf("UNDERDOC_SAMBA_DOMAIN: tenants of UNDERDOC_SAMBA whose companion is a member of a domain, separated by commas; %q has no companion", t)
			}
			cfg.Domain[t] = true
		}
	}
	cfg.DomainNames = env("UNDERDOC_SAMBA_DOMAIN_NAMES", share.DomainByLogin)
	if cfg.DomainNames != share.DomainByLogin && cfg.DomainNames != share.DomainByRemoteID {
		return cfg, false, fmt.Errorf("UNDERDOC_SAMBA_DOMAIN_NAMES: %s or %s", share.DomainByLogin, share.DomainByRemoteID)
	}
	return cfg, true, nil
}

// removeSambaUsers removes every Samba user of a tenant: the migration between app passwords and a domain (T-42). The
// mode of a tenant is chosen at setup and never mixed; to change it, the operator stops UnderDoc, runs this, changes
// UNDERDOC_SAMBA_DOMAIN and the companion, and starts again. The log keeps it.
func removeSambaUsers(ctx context.Context, stdout io.Writer) error {
	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()
	n, err := database.RemoveAllSambaAccounts(ctx)
	if err != nil {
		return err
	}
	t, _ := tenant.From(ctx)
	_, err = fmt.Fprintf(stdout, "%d Samba users of the tenant %s removed; set the mode (UNDERDOC_SAMBA_DOMAIN) and the companion, then start UnderDoc\n", n, t)
	return err
}
