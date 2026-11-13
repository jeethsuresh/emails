"""Sign and notarize the Email app and its Apple Silicon disk image."""
import json
from pathlib import Path
import plistlib
import subprocess
from tempfile import TemporaryDirectory

from electron_macos import TEAM, signing_environment, take_credentials, verify_application

ROOT = Path(__file__).resolve().parents[1]


def run(*args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def package():
    version = json.loads((ROOT / "package.json").read_text())["version"]
    disk = ROOT / "release" / f"Email-{version}-mac-arm64.dmg"
    with signing_environment(take_credentials()) as env:
        run("npx", "--no-install", "electron-builder", "--config", "electron-builder.yml",
            "--mac", "--arm64", "--publish", "never", f"-c.mac.identity=({TEAM})",
            "-c.mac.notarize=true", "-c.forceCodeSigning=true", "-c.dmg.sign=true",
            cwd=ROOT, env=env)
        run("codesign", "--verify", "--strict", disk)
        result = run("xcrun", "notarytool", "submit", disk, "--key", env["APPLE_API_KEY"],
                     "--key-id", env["APPLE_API_KEY_ID"], "--issuer", env["APPLE_API_ISSUER"],
                     "--wait", "--timeout", "20m", "--output-format", "json",
                     capture_output=True, text=True)
        submission = json.loads(result.stdout)
        if submission.get("status") != "Accepted":
            raise ValueError(f"Apple did not accept Email's disk image: {submission.get('id')}")
    run("xcrun", "stapler", "staple", disk)
    run("xcrun", "stapler", "validate", disk)
    with TemporaryDirectory(prefix="email-mac-image-") as temporary:
        mount = Path(temporary)
        run("hdiutil", "attach", "-readonly", "-nobrowse", "-noautoopen", "-mountpoint", mount, disk)
        try:
            app = mount / "Email.app"
            verify_application(app, "com.jeeth.emails")
            info = plistlib.loads((app / "Contents/Info.plist").read_bytes())
            run("lipo", app / "Contents/MacOS" / info["CFBundleExecutable"], "-verify_arch", "arm64")
            run("lipo", app / "Contents/Resources/assets/email-backend", "-verify_arch", "arm64")
        finally:
            run("hdiutil", "detach", mount)


if __name__ == "__main__":
    package()
