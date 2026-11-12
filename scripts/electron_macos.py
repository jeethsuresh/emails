"""Use the pinned Electron packager with a temporary Apple notarization key."""
import base64
from contextlib import contextmanager
import os
from pathlib import Path
import subprocess
from tempfile import TemporaryDirectory

TEAM = "WV3J3848JR"
SECRETS = {
    "CSC_LINK": "MAC_CERTIFICATE_P12_BASE64",
    "CSC_KEY_PASSWORD": "MAC_CERTIFICATE_PASSWORD",
    "APPLE_API_KEY": "MAC_NOTARY_KEY_BASE64",
    "APPLE_API_KEY_ID": "MAC_NOTARY_KEY_ID",
    "APPLE_API_ISSUER": "MAC_NOTARY_ISSUER_ID",
}


def take_credentials():
    credentials = {name: os.environ.pop(secret, "") for name, secret in SECRETS.items()}
    for name, value in credentials.items():
        if not value:
            raise ValueError(f"Set the {SECRETS[name]} signing secret")
    return credentials


@contextmanager
def signing_environment(credentials):
    with TemporaryDirectory(prefix="electron-notary-") as temporary:
        key = Path(temporary) / "notary.p8"
        key.write_bytes(base64.b64decode(credentials["APPLE_API_KEY"], validate=True))
        key.chmod(0o600)
        yield {**os.environ, **credentials, "APPLE_API_KEY": str(key)}


def verify_application(app, bundle_id):
    requirement = (
        f'=anchor apple generic and identifier "{bundle_id}" and certificate leaf[subject.OU] = "{TEAM}"'
        " and certificate leaf[field.1.2.840.113635.100.6.1.13] exists"
    )
    subprocess.run(["codesign", "--verify", "--deep", "--strict", app], check=True)
    subprocess.run(["codesign", "--verify", "-R", requirement, app], check=True)
    subprocess.run(["xcrun", "stapler", "validate", app], check=True)
