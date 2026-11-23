# SPDX-License-Identifier: MIT
"""A real TinySesam for the contract test of UnderDoc's account API and approvals (scripts/check-services.sh).

Runs inside the published TinySesam image (ghcr.io/ollornog/tinysesam, pinned by digest there) in place of its
gateway: TinySesam as a library, configured with the application `underdoc` as docs/accounts.md tells an operator to
configure it. Everything here is made up for the test. The automation key of the service account and the sessions of
the people who confirm are made in this run and written only to the folder of this run (GATE_OUT), which the test reads
and the script removes; they die with the container.

Environment: GATE_BASE (the address the test reaches the gate at), GATE_RETURN (the return address of approvals
registered for UnderDoc), GATE_OUT (a folder the test reads), GATE_PORT.
"""

import json
import os
import secrets
import tempfile

import uvicorn
from fastapi import FastAPI
from tinysesam import TinySesam, TinySesamConfig

BASE = os.environ["GATE_BASE"]
RETURN = os.environ["GATE_RETURN"]
OUT = os.environ["GATE_OUT"]
PORT = int(os.environ.get("GATE_PORT", "8000"))
PASSWORD = secrets.token_urlsafe(18)

OBJECT = {"type": "string", "max": 40, "pattern": "^[0-9a-z]{1,40}$"}
cfg = TinySesamConfig(
    lang="en",
    base_url=BASE,
    db_path=os.path.join(tempfile.mkdtemp(), "gate.db"),
    cookie_secure=False,
    passkey_enabled=False,
    oidc_enabled=False,
    magiclink_enabled=True,
    approvals_enabled=True,
    available_roles=["underdoc-user", "underdoc-admin", "underdoc-delegate", "staff"],
    apps={"underdoc": {
        "account": "svc-underdoc",
        "name": {"en": "UnderDoc", "de": "UnderDoc"},
        "provisioning": {
            "roles": ["underdoc-user"],
            "scopes": ["users:read", "users:invite", "users:update", "users:disable", "users:reset"],
            "approver_role": "underdoc-admin",
            "link_ttl_min": 60,
        },
        "approvals": {
            "return_urls": [RETURN],
            "types": {
                "underdoc.object.purge": {
                    "fields": {"object": OBJECT, "title": {"type": "string", "max": 200}},
                    "title": {"en": "Delete for good"},
                    "summary": {"en": "Delete {title} ({object}) and everything below it for good."},
                    "irreversible": True,
                },
                "underdoc.trash.empty": {
                    "fields": {"items": {"type": "integer", "min": 0, "max": 1000000},
                               "cutoff": {"type": "integer", "min": 0, "max": 9007199254740991}},
                    "title": {"en": "Empty the trash"},
                    "summary": {"en": "Delete {items} items of your trash for good."},
                    "irreversible": True,
                },
            },
        },
    }},
)
auth = TinySesam(cfg)
auth.set_security("password_min_length_single_factor", 8)
app = FastAPI()
app.include_router(auth.router())

service = auth.create_service("svc-underdoc")
key = auth.create_api_key(service, name="underdoc contract test")["key"]
lead = auth.create_user("lead", password=PASSWORD, roles=["underdoc-user"])
boss = auth.create_user("boss", password=PASSWORD, roles=["underdoc-admin"])
outsider = auth.create_user("outsider", password=PASSWORD, roles=["staff"])
# An account the operator hands to UnderDoc (tinysesam accounts adopt): the account API sees it from now on.
adopted = auth.create_user("adopted", password=PASSWORD, roles=["underdoc-user"])
assert auth.store.adopt_account(adopted, "underdoc")


def session(uid):
    return auth.store.create_session(uid, 3600, True, "password", factors=["password"])


facts = {
    "key": key,
    "password": PASSWORD,
    "lead": lead,
    "boss": boss,
    "outsider": outsider,
    "adopted": adopted,
    "service": service,
    "session_cookie": auth.session_cookie_name,
    "csrf_cookie": auth.csrf_cookie_name,
    "sessions": {"lead": session(lead), "boss": session(boss), "outsider": session(outsider)},
}
path = os.path.join(OUT, "gate.json")
# Readable by the test, which runs as another user than the container; the folder belongs to this run alone.
with open(os.open(path + ".tmp", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o644), "w") as f:
    json.dump(facts, f)
os.replace(path + ".tmp", path)

uvicorn.run(app, host="0.0.0.0", port=PORT, log_level="warning")
