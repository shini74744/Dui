#!/usr/bin/env python3
"""Assemble pinned helper binaries into a DUI release's bin directory."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import re
import tarfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parent
PLATFORMS = ('amd64', 'arm64', 'armv7', 'armv6', 'armv5', '386', 's390x')


def fetch(url):
    if not url.startswith('https://github.com/'):
        raise ValueError('helper download must use the pinned GitHub release')
    for attempt in range(3):
        try:
            request = urllib.request.Request(url, headers={'User-Agent': 'DUI-release-builder'})
            with urllib.request.urlopen(request, timeout=60) as response:
                return response.read()
        except OSError:
            if attempt == 2:
                raise
            time.sleep(attempt + 1)


def verify(data, expected):
    if not re.fullmatch('[0-9a-f]{64}', expected) or hashlib.sha256(data).hexdigest() != expected:
        raise ValueError('helper SHA-256 verification failed')


def binary_content(asset, data):
    verify(data, asset['sha256'])
    if asset['kind'] == 'binary':
        return data
    if asset['kind'] != 'tar.gz':
        raise ValueError('unknown helper archive type')
    # Read exactly one regular executable; never extract paths or symlinks.
    with tarfile.open(fileobj=io.BytesIO(data), mode='r:gz') as archive:
        members = [m for m in archive.getmembers() if m.isfile() and Path(m.name).name == asset['member']]
        if len(members) != 1:
            raise ValueError('helper executable missing or ambiguous')
        return archive.extractfile(members[0]).read()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--platform', required=True, choices=PLATFORMS)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--cache', type=Path)
    args = parser.parse_args()
    manifest = json.loads((ROOT / 'manifest.json').read_text())
    args.output.mkdir(parents=True, exist_ok=True)
    status = {}
    sources = []
    goarch = 'arm' if args.platform.startswith('armv') else args.platform
    for name, component in manifest['components'].items():
        license_data = (ROOT / component['license']).read_bytes()
        verify(license_data, component['license_sha256'])
        (args.output / (name + '-LICENSE')).write_bytes(license_data)
        sources.append(name + ' ' + component['tag'] + '\n' + component['source'] + '\n')
        asset = component['platforms'].get(args.platform)
        if asset is None:
            status[name] = {'available': False, 'reason': 'No compatible upstream release for ' + args.platform}
            continue
        cache_file = args.cache / asset['sha256'] if args.cache else None
        if cache_file and cache_file.is_file():
            data = cache_file.read_bytes()
        else:
            data = fetch(asset['url'])
        binary = binary_content(asset, data)
        if cache_file:
            cache_file.parent.mkdir(parents=True, exist_ok=True)
            cache_file.write_bytes(data)
        destination = args.output / (name + '-linux-' + goarch)
        temporary = destination.with_suffix('.new')
        temporary.write_bytes(binary)
        temporary.chmod(0o755)
        os.replace(temporary, destination)
        status[name] = {'available': True, 'tag': component['tag'], 'source': component['source'],
                        'archive_sha256': asset['sha256'], 'binary_sha256': hashlib.sha256(binary).hexdigest()}
    (args.output / 'helpers-BUILD.json').write_text(json.dumps({'platform': args.platform, 'components': status}, indent=2) + '\n')
    (args.output / 'helpers-SOURCES.txt').write_text('\n'.join(sources))
    print(json.dumps({'platform': args.platform, 'components': {n: s['available'] for n, s in status.items()}}))


if __name__ == '__main__':
    main()
