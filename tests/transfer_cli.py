"""Real curl/wget acceptance; fixtures and child processes are always cleaned."""
import hashlib
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[1]

def run(*args):
    return subprocess.run(args, check=True, capture_output=True, timeout=120).stdout

def checksum(path):
    with open(path, "rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()

def main():
    curl = shutil.which("curl.exe" if os.name == "nt" else "curl")
    if not curl:
        raise RuntimeError("curl is required")
    wget = shutil.which("wget") if os.name != "nt" else None
    with tempfile.TemporaryDirectory(prefix="okserver-tools-") as temporary:
        temp = Path(temporary)
        binary = temp / ("okserver.exe" if os.name == "nt" else "okserver")
        run("go", "build", "-o", str(binary), str(ROOT / "cmd" / "okserver"))
        source = temp / "source.bin"
        with source.open("wb") as output:
            block = bytes(range(256)) * 4096
            for _ in range(32): output.write(block)
        expected = checksum(source)
        config = temp / "server.yaml"
        config.write_text("upload:\n  min_free_disk_bytes: 0\n", encoding="utf-8")
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        base = f"http://127.0.0.1:{port}"
        server = subprocess.Popen([str(binary), "-c", str(config), "-s", f"127.0.0.1:{port}", "-p", str(temp / "storage")], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        def head(path):
            try:
                with urllib.request.urlopen(urllib.request.Request(base + path, method="HEAD"), timeout=5) as response:
                    return response.headers
            except urllib.error.HTTPError as error:
                detail = error.read().decode("utf-8", errors="replace")
                raise AssertionError(f"HEAD {path} returned HTTP {error.code}: {detail}") from error
        def completed(name):
            for _ in range(600):
                headers = head("/upload/" + name)
                if headers.get("Upload-State") == "completed":
                    assert headers.get("X-File-SHA256") == expected
                    return
                if headers.get("Upload-State") in ("failed", "canceled"): raise AssertionError(headers)
                time.sleep(.05)
            raise AssertionError("verification timeout")
        try:
            for attempt in range(200):
                try:
                    head("/healthz")
                    break
                except OSError: time.sleep(.05)
            else: raise AssertionError("server startup failed")
            # Default curl media type is accepted. Query SHA-256 takes priority.
            run(curl, "--fail-with-body", "--silent", "--data-binary", "@" + str(source), "-H", "X-File-SHA256: invalid", base + "/upload/curl.bin?sha256=" + expected)
            completed("curl.bin")
            run(curl, "--fail-with-body", "--silent", "-F", "file=@" + str(source), base + "/upload/form.bin")
            completed("form.bin")
            # curl's native -C offset -T sends Content-Range to the same URL.
            first = temp / "prefix.bin"
            with source.open("rb") as source_file: first.write_bytes(source_file.read(1 << 20))
            run(curl, "--fail-with-body", "--silent", "-X", "PUT", "--data-binary", "@" + str(first), "-H", f"Content-Range: bytes 0-{(1 << 20)-1}/{source.stat().st_size}", base + "/upload/resume.bin")
            run(curl, "--fail-with-body", "--silent", "-C", str(1 << 20), "-T", str(source), base + "/upload/resume.bin?sha256=" + expected)
            completed("resume.bin")
            downloaded = temp / "download.bin"
            shutil.copyfile(first, downloaded)
            run(curl, "--fail-with-body", "--silent", "-C", "-", "-o", str(downloaded), base + "/download/curl.bin")
            assert checksum(downloaded) == expected
            if wget:
                run(wget, "-qO", "-", "--post-file=" + str(source), base + "/upload/wget.bin?sha256=" + expected)
                completed("wget.bin")
                shutil.copyfile(first, downloaded)
                run(wget, "-q", "-c", "-O", str(downloaded), base + "/download/wget.bin")
                assert checksum(downloaded) == expected
            print("PASS: 32 MiB curl raw POST, multipart, PUT resume, Range download, SHA-256" + (", wget POST and resume" if wget else "; wget tested on WSL"))
        finally:
            server.terminate()
            try: server.wait(timeout=10)
            except subprocess.TimeoutExpired:
                server.kill(); server.wait(timeout=5)

if __name__ == "__main__": main()
