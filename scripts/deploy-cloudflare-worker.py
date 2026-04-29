#!/usr/bin/env python3
import argparse
import json
import os
import sys
import urllib.error
import urllib.request
import uuid
from pathlib import Path


API_BASE = "https://api.cloudflare.com/client/v4"


def request_json(method, url, token, body=None, content_type="application/json"):
    data = None
    if body is not None:
        if isinstance(body, (dict, list)):
            data = json.dumps(body).encode("utf-8")
        else:
            data = body
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", f"Bearer {token}")
    if data is not None and content_type:
        req.add_header("Content-Type", content_type)
        req.add_header("Content-Length", str(len(data)))
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            return json.load(resp)
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"Cloudflare API {method} {url} failed: HTTP {exc.code}: {detail}") from exc


def multipart_module_upload(worker_path, compatibility_date):
    boundary = "----zai2api" + uuid.uuid4().hex
    metadata = {
        "main_module": worker_path.name,
        "compatibility_date": compatibility_date,
    }
    parts = []

    def add_part(name, content, filename=None, content_type="application/octet-stream"):
        headers = [f"--{boundary}"]
        disposition = f'form-data; name="{name}"'
        if filename:
            disposition += f'; filename="{filename}"'
        headers.append(f"Content-Disposition: {disposition}")
        headers.append(f"Content-Type: {content_type}")
        headers.append("")
        parts.append("\r\n".join(headers).encode("utf-8") + b"\r\n" + content + b"\r\n")

    add_part("metadata", json.dumps(metadata).encode("utf-8"), content_type="application/json")
    add_part(
        worker_path.name,
        worker_path.read_bytes(),
        filename=worker_path.name,
        content_type="application/javascript+module",
    )
    body = b"".join(parts) + f"--{boundary}--\r\n".encode("utf-8")
    return body, f"multipart/form-data; boundary={boundary}"


def deploy(args):
    token = args.api_token or os.getenv("CLOUDFLARE_API_TOKEN")
    account_id = args.account_id or os.getenv("CLOUDFLARE_ACCOUNT_ID")
    script_name = args.script_name or os.getenv("CLOUDFLARE_WORKER_NAME")
    worker_path = Path(args.worker).resolve()

    if not token:
        raise SystemExit("missing API token: pass --api-token or set CLOUDFLARE_API_TOKEN")
    if not account_id:
        raise SystemExit("missing account id: pass --account-id or set CLOUDFLARE_ACCOUNT_ID")
    if not script_name:
        raise SystemExit("missing script name: pass --script-name or set CLOUDFLARE_WORKER_NAME")
    if not worker_path.exists():
        raise SystemExit(f"worker file not found: {worker_path}")

    body, content_type = multipart_module_upload(worker_path, args.compatibility_date)
    upload_url = f"{API_BASE}/accounts/{account_id}/workers/scripts/{script_name}"
    result = request_json("PUT", upload_url, token, body=body, content_type=content_type)
    if not result.get("success"):
        raise SystemExit(json.dumps(result, ensure_ascii=False, indent=2))

    print(f"Uploaded Worker script: {script_name}")

    if args.enable_subdomain:
        subdomain_url = f"{upload_url}/subdomain"
        subdomain_result = request_json("POST", subdomain_url, token, body={"enabled": True})
        if not subdomain_result.get("success"):
            raise SystemExit(json.dumps(subdomain_result, ensure_ascii=False, indent=2))
        print("Enabled workers.dev subdomain")


def main():
    parser = argparse.ArgumentParser(description="Deploy worker.js to Cloudflare Workers.")
    parser.add_argument("--account-id", help="Cloudflare account id. Env: CLOUDFLARE_ACCOUNT_ID")
    parser.add_argument("--api-token", help="Cloudflare API token. Env: CLOUDFLARE_API_TOKEN")
    parser.add_argument("--script-name", help="Worker script name. Env: CLOUDFLARE_WORKER_NAME")
    parser.add_argument("--worker", default="worker.js", help="Worker module file path. Default: worker.js")
    parser.add_argument("--compatibility-date", default="2026-04-29")
    parser.add_argument("--enable-subdomain", action="store_true", help="Enable the workers.dev subdomain")
    args = parser.parse_args()
    deploy(args)


if __name__ == "__main__":
    main()
