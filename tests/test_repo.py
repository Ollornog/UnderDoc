#!/usr/bin/env python3
"""Repository hygiene from the shared kit.

The code is Go (scripts/_go_check.sh), the hygiene comes from the kit's Python base. Excluded checks carry their
reason in the call: leaving one out is allowed, hiding it is not. Runs with the system interpreter, stdlib only.
"""
import hashlib
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT / "tests"))
from _kit import automerge, backlog, golang, hygiene, manifest  # noqa: E402
from _kit.report import Report  # noqa: E402

r = Report("Repository hygiene")
POLICY = hygiene.lade_policy()
FILES = hygiene.getrackte_dateien(str(ROOT))
PROJECTS = ["UnderDoc", "underdoc"]


def check(label, findings):
    r.check(label, not findings, " | ".join(findings[:3]))


check(f"tests/_kit unchanged (kit {manifest.version(str(ROOT))}; otherwise: repokit sync .)", manifest.pruefe(str(ROOT)))
check("file list is complete", hygiene.pruefe_dateiliste_plausibel(FILES, root=str(ROOT)))

# ---- Version: version.go and CHANGELOG must agree
source = (ROOT / "internal/version/version.go").read_text(encoding="utf-8")
match = re.search(r'^const Version = "([^"]+)"', source, re.M)
version = match.group(1) if match else ""
r.check("version.go names a SemVer version", bool(re.fullmatch(r"\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?", version)), version)
changelog = (ROOT / "CHANGELOG.md").read_text(encoding="utf-8")
released = re.findall(r"^## \[(\d+\.\d+\.\d+[^\]]*)\]", changelog, re.M)
if released:
    r.check("version.go matches the newest CHANGELOG release", released[0] == version, f"{version} != {released[0]}")
else:
    r.check("CHANGELOG has [Unreleased] before the first release", "## [Unreleased]" in changelog)

# ---- No map of the network, no secrets, no artifacts
check("no private infrastructure", hygiene.pruefe_private_infrastruktur(str(ROOT), FILES, POLICY, PROJECTS))
check("no secrets", hygiene.pruefe_geheimnisse(str(ROOT), FILES, POLICY))
check("no build artifacts", hygiene.pruefe_artefakte(FILES, POLICY))
check("only neutral addresses", hygiene.pruefe_adressen(str(ROOT), FILES, POLICY, zusaetzliche_hosts=[
    r"keepachangelog\.com", r"semver\.org"]))
# Reviewed by hand, not generated: a list that follows along would wave the next real customer name through.
check("no unknown bare hostnames", hygiene.pruefe_blanke_adressen(str(ROOT), FILES, POLICY, grundstock=[
    "golang.org", "honnef.co", "python.org", "devguide.python.org", "pkg.go.dev", "modernc.org",
    "vuln.go.dev",  # named in a comment of the kit's Go gate
    "draw.io"]))    # a product name in the concept and in M-10
check("nothing is loaded from third parties", hygiene.pruefe_keine_fremdressourcen(str(ROOT), FILES, POLICY))
check("every page has a favicon", hygiene.pruefe_favicon(str(ROOT), FILES, POLICY))

# ---- Names that must never appear in this public repository
# Stored as sha256(name)[:16], so this list does not publish what it protects. The kit's check did not catch these.
FORBIDDEN_NAMES_SHA256_16 = {"4cd67b11c3fc6f92"}


def forbidden_names():
    found = []
    for rel in FILES:
        text = (ROOT / rel).read_text(encoding="utf-8", errors="replace").lower()
        if any(hashlib.sha256(word.encode()).hexdigest()[:16] in FORBIDDEN_NAMES_SHA256_16
               for word in set(re.findall(r"[a-z0-9]+", text))):
            found.append(rel)
    return found


check("no forbidden names", forbidden_names())

# ---- Workflows
check("actions pinned by SHA", hygiene.pruefe_actions_sha_gepinnt(str(ROOT), FILES))
check("every workflow sets permissions", hygiene.pruefe_workflow_permissions(str(ROOT), FILES))
check("no self-hosted runner", hygiene.pruefe_kein_self_hosted_runner(str(ROOT), FILES))
check("every checkout sets persist-credentials: false", hygiene.pruefe_persist_credentials(str(ROOT), FILES))
check("no unconditional cancel-in-progress on main",
      hygiene.pruefe_kein_abbruch_auf_default_branch(str(ROOT), FILES, "main"))
check("vulnerability gate present", hygiene.pruefe_audit_workflow(str(ROOT), FILES))
check("publishing depends on the tag", hygiene.pruefe_veroeffentlichen_am_tag(str(ROOT)))
check("Dependabot auto-merge follows the template", automerge.pruefe(str(ROOT)))

# ---- Files
check("scripts are executable", hygiene.pruefe_ausfuehrbar(str(ROOT), [
    "scripts/check.sh", "scripts/_go_check.sh", "scripts/_residue_check.sh", "tests/test_repo.py"]))
check("required files present", hygiene.pruefe_pflichtdateien(str(ROOT), [
    "LICENSE", "README.md", "i18n/README.de.md", "CHANGELOG.md", "SECURITY.md", "i18n/SECURITY.de.md",
    "CODE_OF_CONDUCT.md", "i18n/CODE_OF_CONDUCT.de.md", "CONTRIBUTING.md", "i18n/CONTRIBUTING.de.md",
    "docs/concept.md", ".gitignore", ".github/dependabot.yml", ".github/audit/go.mod",
    ".github/workflows/ci.yml", ".github/workflows/audit.yml", ".github/workflows/release.yml",
    "go.mod", "internal/version/version.go", "scripts/check.sh", "scripts/_residue_check.sh",
    "scripts/_backlog.py", "tests/_kit/hygiene.py", "tests/_kit/backlog.py", "backlog/README-KONVENTION.md"]))
check("CHANGELOG follows Keep a Changelog", hygiene.pruefe_changelog_kategorien(str(ROOT), POLICY))
check("translations keep the shape of the original", hygiene.pruefe_uebersetzungs_struktur(str(ROOT), [
    ("README.md", "i18n/README.de.md"), ("SECURITY.md", "i18n/SECURITY.de.md"),
    ("CONTRIBUTING.md", "i18n/CONTRIBUTING.de.md")]))

# GitHub's community detector compares the file with the template and tolerates no foreign content.
coc = (ROOT / "CODE_OF_CONDUCT.md").read_text(encoding="utf-8")
r.check("CODE_OF_CONDUCT.md stays pure", coc.startswith("# Contributor Covenant Code of Conduct")
        and not any(x in coc for x in ("<img", "<p align", "Deutsch", "+++")))
r.check("CODE_OF_CONDUCT.md names the contact", "underdoc-github@ollornog.de" in coc)

# ---- Go variant of the kit
check("check tools pinned in .github/audit/go.mod", golang.pruefe_go_werkzeuge(str(ROOT)))
check("Dependabot covers every go.mod and Dockerfile", golang.pruefe_dependabot_go(str(ROOT), FILES))
check("check.sh calls the Go part of the gate", golang.pruefe_go_check_verdrahtet(str(ROOT)))

# ---- Text, backlog, runners, policy
check("texts: links, anchors, front matter", hygiene.pruefe_texte(str(ROOT), FILES))
check("text fast path wired into check.sh", hygiene.pruefe_nur_text(str(ROOT)))
check("backlog is structurally sound", backlog.alle_pruefungen(str(ROOT)))
r.check("backlog has entries", bool(backlog.lade(str(ROOT))))
check("every test file is called by a runner", hygiene.pruefe_testdateien_gerufen(str(ROOT)))
check("parallel workers come from CI_KERNE", hygiene.pruefe_parallel_worker(str(ROOT), FILES))
check("citation patterns hit no code", hygiene.pruefe_belegstellen_eng(str(ROOT), FILES, []))
check("every kit check is in exactly one list", hygiene.pruefe_tabelle_vollstaendig())
check("every policy key is read", hygiene.pruefe_policy_schluessel_gelesen(POLICY))

check("every kit check is called or excluded with a reason", hygiene.pruefe_kit_prueffunktionen_gerufen(str(ROOT), ausgenommen={
    "pruefe_run_all_sammelt_automatisch": "no run_all.py: the tests are Go, hygiene is this one script",
    "pruefe_python_matrix": "Go repository, no Python matrix",
    "pruefe_python_matrix_regel": "Go repository, no Python matrix",
    "pruefe_requires_python": "no pyproject.toml",
    "pruefe_extras_imports": "no pyproject.toml, no extras",
    "pruefe_versionsgleichstand": "the version is in internal/version/version.go and is checked above",
}))
check("every kit check saw something", hygiene.pruefe_etwas_gesehen())

sys.exit(r.done())
