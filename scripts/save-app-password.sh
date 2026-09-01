#!/usr/bin/env bash
# Writes the Gmail account credentials into .secrets.env (gitignored, never
# committed). The app password is read from the clipboard (pbpaste) rather
# than typed on the command line, so it never ends up in shell history.
# Re-run this any time the app password is regenerated/revoked.
set -euo pipefail

USERNAME="natyv.test@gmail.com"
SECRETS_FILE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.secrets.env"

# Google displays the app password as 4 groups of 4 letters, separated by
# U+00A0 non-breaking spaces (not regular ASCII spaces -- `tr -d
# '[:space:]'` doesn't catch those). The password itself is always exactly
# 16 lowercase letters, so keep only a-z/A-Z and drop everything else --
# robust regardless of which separator character actually got copied.
APP_PASSWORD="$(pbpaste | tr -cd 'a-zA-Z')"

if [ -z "$APP_PASSWORD" ]; then
	echo "error: clipboard is empty -- copy the app password first" >&2
	exit 1
fi

if [ "${#APP_PASSWORD}" -ne 16 ]; then
	echo "warning: clipboard content is ${#APP_PASSWORD} chars, not the expected 16 -- writing it anyway, but double check it's really the app password" >&2
fi

cat >"$SECRETS_FILE" <<EOF
GMAIL_USERNAME=$USERNAME
GMAIL_APP_PASSWORD=$APP_PASSWORD
EOF

echo "wrote $SECRETS_FILE (gitignored) -- GMAIL_USERNAME=$USERNAME, GMAIL_APP_PASSWORD=<${#APP_PASSWORD} chars>"
