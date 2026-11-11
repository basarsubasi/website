#!/usr/bin/env python3
"""Exercise the production image using disposable containers, volumes, and S3.

Requires Docker and Python 3; no cloud credentials or running app are used.
"""
import json
import pathlib
import struct
import subprocess
import time
import urllib.error
import urllib.request
import uuid
import zlib

ROOT = pathlib.Path(__file__).resolve().parent.parent
PASSWORD = "container-smoke-password"


def docker(*args, check=True):
    result = subprocess.run(["docker", *args], cwd=ROOT, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            timeout=600 if args[0] == "build" else 60)
    if check and result.returncode:
        raise RuntimeError(f"docker {args[0]} failed:\n{result.stderr}")
    return result


def request(base, path, payload=None, cookie=None, method=None, content_type=None):
    headers = {}
    if isinstance(payload, dict):
        payload = json.dumps(payload).encode()
        content_type = "application/json"
    if content_type:
        headers["Content-Type"] = content_type
    if cookie:
        headers["Cookie"] = cookie
    req = urllib.request.Request(base + path, data=payload, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=3) as response:
        return response.status, response.headers, response.read()


def wait_for(check, description):
    deadline = time.monotonic() + 40
    last_error = None
    while time.monotonic() < deadline:
        try:
            if check():
                return
        except (OSError, ValueError, KeyError) as error:
            last_error = error
        time.sleep(0.25)
    raise RuntimeError(f"Timed out waiting for {description}: {last_error}")


def png():
    def chunk(kind, data):
        return struct.pack("!I", len(data)) + kind + data + struct.pack("!I", zlib.crc32(kind + data))
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack("!2I5B", 1, 1, 8, 2, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(b"\x00\xff\x00\x00")) + chunk(b"IEND", b""))


def main():
    name = "website-smoke-" + uuid.uuid4().hex[:10]
    image, fixture_image = name + ":app", name + ":fixture"
    app, restored, storage = name + "-app", name + "-restored", name + "-s3"
    volumes = [name + "-data", name + "-restore"]
    containers = [app, restored, storage]
    try:
        print("Building production and S3 fixture images…", flush=True)
        docker("build", "--target", "production", "-t", image, ".")
        docker("build", "--target", "test-fixture", "-t", fixture_image, ".")
        config = json.loads(docker("image", "inspect", image).stdout)[0]["Config"]
        assert config["User"] == "10001:10001", config["User"]
        assert config["Entrypoint"] == ["/usr/local/bin/website"]
        docker("network", "create", name)
        for volume in volumes:
            docker("volume", "create", volume)
        docker("run", "-d", "--name", storage, "--network", name, "--network-alias", "fixture",
               "-p", "127.0.0.1::9000", fixture_image)

        def address(container, port):
            return "http://" + docker("port", container, f"{port}/tcp").stdout.strip()

        s3 = address(storage, 9000)

        def start(container, volume):
            docker("run", "-d", "--name", container, "--network", name,
                   "--read-only", "--tmpfs", "/tmp", "--cap-drop=ALL",
                   "--security-opt", "no-new-privileges", "-p", "127.0.0.1::8080",
                   "-v", f"{volume}:/data",
                   "-e", f"ADMIN_PASSWORD={PASSWORD}", "-e", "R2_ENDPOINT=http://fixture:9000",
                   "-e", "R2_BUCKET=test", "-e", "R2_ACCESS_KEY_ID=test", "-e", "R2_SECRET_ACCESS_KEY=test",
                   "-e", f"R2_PUBLIC_URL={s3}/test", "-e", "COOKIE_SECURE=true",
                   "-e", "TRUST_FLY_PROXY=true", "-e", "NO_PROXY=fixture", "-e", "no_proxy=fixture", image)
            base = address(container, 8080)
            wait_for(lambda: request(base, "/api/health")[0] == 200, "healthy application")
            return base

        base = start(app, volumes[0])
        assert "10001" in docker("top", app, "-eo", "pid,uid").stdout.splitlines()[1:][0]
        assert docker("exec", app, "/bin/sh", "-c", "true", check=False).returncode != 0
        assert b"<html" in request(base, "/")[2]
        _, headers, _ = request(base, "/api/auth/login", {"password": PASSWORD})
        assert "Secure" in headers["Set-Cookie"] and "HttpOnly" in headers["Set-Cookie"]
        cookie = headers["Set-Cookie"].split(";", 1)[0]
        status, _, body = request(base, "/api/admin/posts", {
            "slug": "persistent", "title": "Persisted", "body": "Survives restart", "published": 1,
        }, cookie)
        assert status == 201
        post = json.loads(body)["data"]
        boundary = "smoke-boundary"
        multipart = (f'--{boundary}\r\nContent-Disposition: form-data; name="metadata"\r\n\r\n'
                     '{"name":"Smoke photo","color":"bw"}\r\n'
                     f'--{boundary}\r\nContent-Disposition: form-data; name="photo"; filename="photo.png"\r\n'
                     'Content-Type: image/png\r\n\r\n').encode() + png() + f"\r\n--{boundary}--\r\n".encode()
        status, _, body = request(base, "/api/admin/images", multipart, cookie,
                                  content_type=f"multipart/form-data; boundary={boundary}")
        assert status == 201
        photo = json.loads(body)["data"]

        def backed_up():
            catalog = json.loads(request(s3, "/test/catalog.json")[2])
            return post["id"] in catalog["posts"] and photo["uuid"] in catalog["images"]

        wait_for(backed_up, "background worker to back up the post and image")
        assert request(s3, "/test/" + photo["object_key"])[2] == png()
        docker("stop", "--time", "15", app)
        assert docker("inspect", "--format", "{{.State.ExitCode}}", app).stdout.strip() == "0"
        base = start(restored, volumes[1])
        assert json.loads(request(base, "/api/posts/persistent")[2])["data"] == post
        assert json.loads(request(base, "/api/images/" + photo["uuid"])[2])["data"] == photo
        docker("stop", storage)
        docker("restart", restored)
        base = address(restored, 8080)
        wait_for(lambda: request(base, "/api/health")[0] == 200, "restart without R2")
        assert json.loads(request(base, "/api/posts/persistent")[2])["data"] == post
        assert json.loads(request(base, "/api/images/" + photo["uuid"])[2])["data"] == photo
        docker("stop", "--time", "15", restored)
        assert docker("inspect", "--format", "{{.State.ExitCode}}", restored).stdout.strip() == "0"
        print("PASS: non-root/shell-free image, fresh volume, UI, secure login, photo upload, "
              "background backup, fresh-DB restore, offline restart, graceful shutdown")
    except BaseException:
        for container in containers:
            logs = docker("logs", container, check=False)
            if logs.returncode == 0:
                print(f"--- {container} ---\n{logs.stdout}{logs.stderr}")
        raise
    finally:
        for container in containers:
            docker("rm", "-f", "-v", container, check=False)
        for volume in volumes:
            docker("volume", "rm", volume, check=False)
        docker("network", "rm", name, check=False)
        docker("image", "rm", image, fixture_image, check=False)


if __name__ == "__main__":
    main()
