#!/usr/bin/env python3
"""Repository hygiene from the shared kit.

The code is Go (scripts/_go_check.sh), the hygiene comes from the kit's Python base. Excluded checks carry their
reason in the call: leaving one out is allowed, hiding it is not. Runs with the system interpreter, stdlib only.
"""
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
    r"keepachangelog\.com", r"semver\.org",
    r"tailwindcss\.com",  # the license comment in the compiled style pack
    r"scripts\.sil\.org",  # the license text of the font, which has to stay unchanged
    r"deb\.debian\.org",  # the Debian backports Samba comes from (deploy/samba/Dockerfile)
]))
# Reviewed by hand, not generated: a list that follows along would wave the next real customer name through.
check("no unknown bare hostnames", hygiene.pruefe_blanke_adressen(str(ROOT), FILES, POLICY, grundstock=[
    "golang.org", "honnef.co", "python.org", "devguide.python.org", "pkg.go.dev", "modernc.org",
    "vuln.go.dev",  # named in a comment of the kit's Go gate
    "gcr.io",   # the base image in the Dockerfile
    "ghcr.io",  # where .github/workflows/image.yml publishes the image
    "draw.io"]))    # a product name in the concept and in M-10
check("nothing is loaded from third parties", hygiene.pruefe_keine_fremdressourcen(str(ROOT), FILES, POLICY))
check("every page has a favicon", hygiene.pruefe_favicon(str(ROOT), FILES, POLICY))

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


def search_engine_pins():
    """The services of ci.yml run the search engines the tests know: the same tag and digest as scripts/search/Dockerfile.*.
    Dependabot moves the Dockerfiles; the services of a workflow it does not, so a moved pin turns this red until ci.yml
    follows."""
    found = []
    pinned = {}
    for path in sorted((ROOT / "scripts/search").glob("Dockerfile.*")):
        m = re.search(r"^FROM (\S+)", path.read_text(encoding="utf-8"), re.M)
        if m:
            pinned[m.group(1).split(":")[0]] = m.group(1)
    workflow = (ROOT / ".github/workflows/ci.yml").read_text(encoding="utf-8")
    services = re.search(r"^    services:\n((?:      .*\n|\s*\n)+)", workflow, re.M)
    images = re.findall(r"^        image: (\S+)", services.group(1), re.M) if services else []
    if not pinned:
        found.append("no pin in scripts/search/Dockerfile.*")
    for name, pin in pinned.items():
        if pin not in images:
            found.append(f"ci.yml has no service {pin}")
    for image in images:
        if image not in pinned.values():
            found.append(f"ci.yml runs {image}, pinned in no scripts/search/Dockerfile.*")
    return found


check("the search services of ci.yml are the pins of scripts/search", search_engine_pins())


def opensearch_ignores_the_disk():
    """The OpenSearch of the tests switches its disk watermarks off: above 90 % of the host's disk it refuses to create
    an index, so a test would turn red because the machine is full. The deployment template keeps the protection."""
    setting = "cluster.routing.allocation.disk.threshold_enabled"
    found = []
    script = (ROOT / "scripts/search-engines.sh").read_text(encoding="utf-8")
    if f"-e {setting}=false" not in script:
        found.append(f"scripts/search-engines.sh does not start OpenSearch with {setting}=false")
    workflow = (ROOT / ".github/workflows/ci.yml").read_text(encoding="utf-8")
    if not re.search(rf'^          {re.escape(setting)}: "false"$', workflow, re.M):
        found.append(f"the OpenSearch service of ci.yml does not set {setting}: \"false\"")
    if setting in (ROOT / "deploy/compose.yml").read_text(encoding="utf-8"):
        found.append(f"deploy/compose.yml touches {setting}: production keeps the disk watermarks")
    return found


check("the OpenSearch of the tests does not depend on the host's disk", opensearch_ignores_the_disk())

# ---- Files
check("scripts are executable", hygiene.pruefe_ausfuehrbar(str(ROOT), [
    "scripts/check.sh", "scripts/_go_check.sh", "scripts/_residue_check.sh", "scripts/build-js.sh", "tests/test_repo.py"]))
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
