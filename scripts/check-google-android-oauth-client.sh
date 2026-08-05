#!/usr/bin/env bash
# Fails if Firebase Android app for app.codexlauncher lacks an oauth_client with
# the debug SHA-1. Prevents the AccountReauth "not registered" failure mode.
set -euo pipefail
PROJECT="${GOOGLE_CLOUD_PROJECT:-operator-504223}"
PACKAGE="${ANDROID_PACKAGE:-app.codexlauncher}"
SHA1_HEX="${ANDROID_DEBUG_SHA1_HEX:-f7a85af51de481aa5c1d1211c0f18d1b0683c57f}"
TOKEN="$(gcloud auth print-access-token)"
APPS_JSON="$(curl -sS -H "Authorization: Bearer ${TOKEN}" -H "x-goog-user-project: ${PROJECT}" \
  "https://firebase.googleapis.com/v1beta1/projects/${PROJECT}/androidApps")"
APP_NAME="$(printf '%s' "$APPS_JSON" | python3 -c "import sys,json; apps=json.load(sys.stdin).get('apps',[]);
print(next(a['name'] for a in apps if a.get('packageName')==sys.argv[1]))" "$PACKAGE")"
CFG_JSON="$(curl -sS -H "Authorization: Bearer ${TOKEN}" -H "x-goog-user-project: ${PROJECT}" \
  "https://firebase.googleapis.com/v1beta1/${APP_NAME}/config")"
printf '%s' "$CFG_JSON" | python3 -c "
import base64, json, sys
package=sys.argv[1]
want=sys.argv[2].lower()
d=json.load(sys.stdin)
j=json.loads(base64.b64decode(d['configFileContents']).decode())
clients=j['client'][0].get('oauth_client') or []
ok=any(
  c.get('client_type')==1 and
  (c.get('android_info') or {}).get('package_name')==package and
  (c.get('android_info') or {}).get('certificate_hash','').lower()==want
  for c in clients
)
print('oauth_client_count', len(clients))
print('package', package)
print('sha1', want)
print('registered', ok)
sys.exit(0 if ok else 1)
" "$PACKAGE" "$SHA1_HEX"
