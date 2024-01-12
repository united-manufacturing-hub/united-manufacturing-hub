import os
import re
import requests

def go_through_files_and_folders(start_path):
    f = []
    for root, dirs, files in os.walk(start_path):
        dirs[:] = [d for d in dirs if not d[0] == '.' and d not in ['venv', 'vendor']]
        for file in files:
            if not file.startswith('.') and file not in [
                "go.mod",
                "go.sum",
                "go.work",
                "go.work.sum",
                "find_urls.py",
                "LICENSE",
                "NOTICE",
                "CODEOWNERS",
                "Dockerfile"
            ] and not file.endswith(".go") and not file.endswith(".md"):
                f.append(os.path.join(root, file))
    return f

allowed_urls = [
    "http://www.apache.org/licenses/LICENSE-2.0",
    "https://umh.app",
    "https://www.umh.app",
    "http://www.apache.org/licenses/",
    "http://www.openssl.org",
    "https://github.com/united-manufacturing-hub/united-manufacturing-hub",
    "https://semver.org",
    "http://example.net/pki/my_ca.crl",
    "https://github.com/hivemq/hivemq-file-rbac-extension",
    "http://www.w3.org/2001/XMLSchema-instance",
    "https://wanderingdeveloper.medium.com/reusing-auto-generated-helm-secrets-a7426403d4bb",
    "https://gist.github.com/fphilipe/0a2a3d50a9f3834683bf",
]

def finder(file):
    with open(file, 'r') as f:
        try:
            data = f.read()
        except UnicodeDecodeError:
            return [],[]
        url_pattern = re.compile(r'http[s]?://(?:[a-zA-Z]|[0-9]|[$-_@.&+]|[!*\\(\\),]|(?:%[0-9a-fA-F][0-9a-fA-F]))+')
        urls = re.findall(url_pattern, data)
        urls = [url for url in urls if url not in allowed_urls]

        # Ignore prerelease url's (prerelease[0-9]*.tgz
        urls = [url for url in urls if not re.search(r'prerelease[0-9]*', url)]


        mgmturls = []
        otherurls = []
        # Split urls into two pools, beginning with https://management.umh.app and not
        for url in urls:
            # Strip weird stuff at the end (for example, trailing '.', ';')
            url = url.rstrip('.;')

            if url.startswith("https://management.umh.app"):
                mgmturls.append(url)
            else:
                otherurls.append(url)


        return mgmturls, otherurls


files = go_through_files_and_folders('.')

unique_management_urls = []
for file in files:
    mgmturls, otherurls = finder(file)
    for m in mgmturls:
        unique_management_urls.append(m)

    if len(otherurls) > 0:
        print(f"{file}")
        for o in otherurls:
            print(f"\t{o}")

unique_management_urls = list(set(unique_management_urls))

# Test if mgmt urls are reachable
for m in unique_management_urls:
    print(f"Checking {m}")
    # Validate by doing a GET request
    if m.startswith("https://management.umh.app/helm") and not m.endswith("tgz"):
        m = f"{m}/index.yaml"
    responseGet = requests.get(m)
    if responseGet.status_code != 200:
        print(f"\t{m} is not accessible")