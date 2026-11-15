#!/usr/bin/python3
"""List account-scoped model catalogs from the local Antigravity proxy."""
import argparse
from concurrent.futures import ThreadPoolExecutor
import ipaddress
import json
from pathlib import Path
import sys
import urllib.error
import urllib.parse
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, message, headers, newurl):
        return None


OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())


def fetch_json(url, headers):
    with OPENER.open(urllib.request.Request(url, headers=headers), timeout=30) as response:
        return json.load(response)


def read_catalog(url, headers):
    models, token, seen = set(), "", set()
    while True:
        page_url = url + ("?" + urllib.parse.urlencode({"pageToken": token}) if token else "")
        page = fetch_json(page_url, headers)
        for model in page.get("models", []):
            name = model["name"]
            if not isinstance(name, str) or not name.startswith("models/") or name == "models/":
                raise ValueError("invalid model name")
            models.add(name.removeprefix("models/"))
        token = page.get("nextPageToken", "")
        if not token:
            return models
        if not isinstance(token, str) or token in seen:
            raise ValueError("invalid pagination")
        seen.add(token)


def admin_base(base_url):
    url = urllib.parse.urlsplit(base_url)
    host = url.hostname
    local = host == "localhost"
    if not local and host:
        try:
            local = ipaddress.ip_address(host).is_loopback
        except ValueError:
            pass
    if not local or url.scheme not in ("http", "https") or url.username or url.password or url.query or url.fragment:
        raise ValueError("管理查询仅支持本机代理地址，不会向远程地址发送管理密钥")
    return urllib.parse.urlunsplit((url.scheme, url.netloc, "", "", ""))


def failure(error):
    if isinstance(error, urllib.error.HTTPError):
        if error.code == 404:
            return "HTTP 404：请确认代理已更新，支持按账号查询模型"
        if error.code in (401, 403):
            return "HTTP %d：请检查管理密钥和账号是否启用" % error.code
        return "HTTP %d" % error.code
    if isinstance(error, (urllib.error.URLError, TimeoutError)):
        return "连接失败或超时，请检查本机代理和上游连接"
    return "无法读取配置或模型响应"


def main(argv=None, config_dir=None):
    parser = argparse.ArgumentParser(description="列出 AGY 各账号上游公布的模型，默认显示所有已启用账号的并集")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--account", metavar="ID", help="只显示指定账号的模型")
    mode.add_argument("--current", action="store_true", help="仅使用 client.json 的原有 API Key 查询")
    args = parser.parse_args(argv)
    folder = Path(config_dir) if config_dir else Path.home() / ".config/antigravity-proxy"
    try:
        config = json.loads((folder / "client.json").read_text())
        if args.current:
            models = read_catalog(config["base_url"].rstrip("/") + "/models", {"x-goog-api-key": config["api_key"]})
            print("共 %d 个模型（当前客户端密钥绑定账号，上游公布列表）：" % len(models))
            for name in sorted(models):
                print(name)
            return 0
        base = admin_base(config["base_url"])
        secret = (folder / "admin-key").read_text().strip()
        if not secret:
            raise ValueError("missing admin key")
        headers = {"Authorization": "Bearer " + secret}
        accounts = fetch_json(base + "/admin/accounts", headers)
        if not isinstance(accounts, list):
            raise ValueError("invalid account list")
        if args.account and not any(account["id"] == args.account for account in accounts):
            print("未找到指定账号；请用 antigravity-proxy accounts list 查看账号 ID。", file=sys.stderr)
            return 1
        selected = [account["id"] for account in accounts if account.get("enabled") and account.get("credential_configured") and (not args.account or account["id"] == args.account)]
        if not selected:
            print("没有符合条件的已启用、已授权账号。", file=sys.stderr)
            return 1
        def query(account):
            try:
                url = base + "/admin/accounts/" + urllib.parse.quote(account, safe="") + "/models"
                return account, read_catalog(url, headers), None
            except (OSError, ValueError, KeyError, TypeError) as error:
                return account, set(), failure(error)
        memberships, counts, errors = {}, {}, []
        with ThreadPoolExecutor(max_workers=min(4, len(selected))) as pool:
            for account, models, error in pool.map(query, selected):
                if error:
                    errors.append((account, error))
                    continue
                counts[account] = len(models)
                for name in models:
                    memberships.setdefault(name, []).append(account)
        print("共 %d 个模型（%d/%d 个账号查询成功，上游公布列表）：" % (len(memberships), len(counts), len(selected)))
        for account in sorted(counts):
            print("  %s：%d 个" % (account, counts[account]))
        print("每行标注来源账号；调用模型仍需使用对应账号的 API Key。")
        for name in sorted(memberships):
            print("%s  [%s]" % (name, ", ".join(sorted(memberships[name]))))
        for account, error in errors:
            print("账号 %s 查询失败：%s" % (account, error), file=sys.stderr)
        return 1 if errors else 0
    except (OSError, ValueError, KeyError, TypeError) as error:
        print("查询失败：%s。配置目录：~/.config/antigravity-proxy；可用 --current 查询原客户端账号。" % failure(error), file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
