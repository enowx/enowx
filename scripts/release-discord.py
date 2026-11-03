#!/usr/bin/env python3
"""Posts a published release's changelog to the enowX Discord server, once.

It posts as the bot, because Discord lets a role that isn't mentionable be pinged only by a
bot with the Mention Everyone permission. It never fails the release: what goes wrong is a
warning in the log. A release already posted in the channel isn't posted again, so a rerun
is safe.

Environment: DISCORD_RELEASE_BOT_TOKEN, RELEASE_TAG (v1.2.3), RELEASE_URL.
"""
import argparse
import json
import os
import re
import sys
import urllib.error
import urllib.request

API = "https://discord.com/api/v10"
CHANNEL = "1566927939122110464"  # #releases
ROLE = "1566929207387496518"  # Update Notification
COLOR = 0x2F6FED
DESCRIPTION_MOST = 4096  # Discord's limit on an embed's description
HISTORY = 50  # how far back an earlier post of this release is looked for


def warn(message):
    print(f"::warning::{message}")


def section(changelog, version):
    """The changelog's section for version, without its heading."""
    out, inside = [], False
    for line in changelog.splitlines():
        if line.startswith("## "):
            if inside:
                break
            inside = re.fullmatch(rf"## \[?{re.escape(version)}\]?(\s.*)?", line) is not None
            continue
        if inside:
            out.append(line)
    return "\n".join(out).strip()


def description(text, url):
    """text within Discord's limit, cut at the end of a line, with a link to the rest."""
    if len(text) <= DESCRIPTION_MOST:
        return text
    rest = f"\n\n[Read the rest of what changed]({url})"
    cut = text[: DESCRIPTION_MOST - len(rest)]
    cut = cut[: cut.rfind("\n")] if "\n" in cut else cut
    return cut.rstrip() + rest


def message(tag, url, text):
    return {
        "content": f"<@&{ROLE}>",
        "allowed_mentions": {"parse": [], "roles": [ROLE]},
        "embeds": [
            {
                "title": f"enowX {tag}",
                "url": url,
                "description": description(text, url),
                "color": COLOR,
                "footer": {"text": "enowX updates itself: restart it when it offers. Update every host together."},
            }
        ],
    }


def call(method, path, token, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(API + path, data=data, method=method)
    req.add_header("Authorization", f"Bot {token}")
    req.add_header("User-Agent", "enowX-release (https://github.com/enowdev/enowx, 1)")
    if data is not None:
        req.add_header("Content-Type", "application/json")
    with urllib.request.urlopen(req, timeout=30) as res:
        raw = res.read()
    return json.loads(raw) if raw else None


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--dry-run", action="store_true", help="print the message, post nothing")
    parser.add_argument("--channel", default=CHANNEL)
    parser.add_argument("--changelog", default="CHANGELOG.md")
    args = parser.parse_args()

    tag, url = os.environ.get("RELEASE_TAG", ""), os.environ.get("RELEASE_URL", "")
    if not re.fullmatch(r"v\d+\.\d+\.\d+", tag):
        print(f"{tag!r} isn't a release's tag: nothing to post")
        return
    with open(args.changelog, encoding="utf-8") as f:
        text = section(f.read(), tag[1:])
    if not text:
        warn(f"CHANGELOG.md has no section for {tag}: nothing posted")
        return
    body = message(tag, url, text)
    if args.dry_run:
        print(json.dumps(body, indent=2, ensure_ascii=False))
        return
    token = os.environ.get("DISCORD_RELEASE_BOT_TOKEN", "")
    if not token:
        warn("DISCORD_RELEASE_BOT_TOKEN isn't set: nothing posted")
        return
    try:
        for old in call("GET", f"/channels/{args.channel}/messages?limit={HISTORY}", token):
            if any(e.get("title") == body["embeds"][0]["title"] for e in old.get("embeds", [])):
                print(f"{tag} is already posted: nothing to do")
                return
        posted = call("POST", f"/channels/{args.channel}/messages", token, body)
        print(f"posted {tag} as message {posted['id']}, mentioning roles {posted.get('mention_roles')}")
    except (urllib.error.URLError, OSError, ValueError, KeyError) as e:
        # Only the status: nothing of the request, which holds the token, goes in the log.
        warn(f"couldn't post {tag} to Discord: {getattr(e, 'code', type(e).__name__)}")


if __name__ == "__main__":
    sys.exit(main())
