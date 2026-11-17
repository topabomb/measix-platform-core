#!/usr/bin/env python3
"""Disposable, identity-pinned Agent Space fixture for the Core browser harness.

The existing Agent Space qualification owner manages Docker resources. No user,
space, credential or file business operations are performed by this bridge.
"""
import argparse
import hashlib
import http.client
import importlib.util
import json
import os
from pathlib import Path
import re
import secrets
import signal
import subprocess
import sys
import time
import uuid


def load_owner(source):
    directory = source.resolve(strict=True)/'docker'
    verifier = directory/'verify-previews.py'
    sys.path.insert(0, str(directory))
    spec = importlib.util.spec_from_file_location('agentspace_preview_qualification', verifier)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module.FixtureOwner


def docker_command(environment):
    def command(*arguments, timeout=180):
        result = subprocess.run(['docker', *arguments], env=environment, capture_output=True,
                                text=True, timeout=timeout)
        if result.returncode:
            raise RuntimeError('docker '+str(arguments[:2])+' failed: '+result.stderr[-2000:])
        return (result.stdout+result.stderr if arguments[:1] == ('logs',) else result.stdout).strip()
    return command


def run(args):
    if not re.fullmatch(r'sha256:[0-9a-f]{64}', args.image) or any(
            not re.fullmatch(r'[0-9a-f]{64}', item) for item in (args.expected_source, args.expected_helper)):
        raise ValueError('Pass exact image and independently expected source/helper identities')
    args.output = args.output.resolve()
    args.output.mkdir(mode=0o700, parents=False, exist_ok=False)
    report = {'ready': False, 'cleanupPassed': False, 'image': args.image,
              'expectedSource': args.expected_source, 'expectedHelper': args.expected_helper}
    owner = None
    command = None
    name = 'measix-preview-browser-'+uuid.uuid4().hex[:12]
    try:
        owner_type = load_owner(args.source)
        management_token = secrets.token_urlsafe(40)
        environment = dict(os.environ, AGENT_SPACE_ADMIN_TOKEN=management_token)
        command = docker_command(environment)
        image = json.loads(command('image', 'inspect', args.image))[0]
        labels = image['Config']['Labels']
        assert image['Id'] == args.image and image['Architecture'] == 'amd64' and image['Os'] == 'linux'
        assert labels['io.runcove.target'] == 'server'
        assert labels['io.runcove.source-sha256'] == args.expected_source
        owner = owner_type(command, name)
        volume = name+'-data'
        owner.volume(volume)
        config = args.output/'agent-space.toml'
        config.write_text('''resource_prefix="corepreview"
[server]
listen="0.0.0.0:8932"
public_base_url="http://localhost:8932"
max_running_spaces=2
[workspace]
image={preset="runcove-desktop"}
cpus=1
memory_mib=1536
workspace_size_mib=512
tmpfs_size_mib=128
shell="/bin/bash"
exec_timeout_secs=30
[state]
dir="/var/lib/microsandbox/agent-space-state"
[admin]
token_env="AGENT_SPACE_ADMIN_TOKEN"
[webdav]
enabled=true
listen="0.0.0.0:8933"
public_base_url="http://localhost:8933"
startup_timeout_secs=90
''', encoding='utf-8')
        config.chmod(0o600)
        common = ['-e', 'AGENT_SPACE_ADMIN_TOKEN', '--mount', 'type=volume,src='+volume+',dst=/var/lib/microsandbox',
                  '--mount', 'type=bind,src='+str(config)+',dst=/etc/agent-space/config.toml,readonly']
        owner.container(name+'-init', [*common, args.image, 'init-state'])
        command('start', '-a', name+'-init')
        owner.container(name, ['--device', '/dev/kvm', '--stop-timeout', '60', '-p', '127.0.0.1::8932',
                              '-p', '127.0.0.1::8933', *common, args.image])
        command('start', name)
        ports = [int(command('port', name, str(port)+'/tcp').rsplit(':', 1)[1]) for port in (8932, 8933)]
        deadline = time.monotonic()+120
        while True:
            try:
                connection = http.client.HTTPConnection('127.0.0.1', ports[0], timeout=3)
                connection.request('GET', '/healthz')
                response = connection.getresponse()
                status = response.status
                response.read(4096)
                connection.close()
                if status == 200: break
            except (OSError, http.client.HTTPException):
                pass
            if time.monotonic() >= deadline: raise RuntimeError('fixture readiness timeout')
            time.sleep(.3)
        release = json.loads(command('exec', name, 'cat', '/opt/agent-space/release.json'))
        assert release['target'] == 'server' and release['source_sha256'] == args.expected_source
        if getattr(args, 'expected_workspace', None):
            assert release['workspace']['reference'].endswith('@'+args.expected_workspace)
        actual = command('exec', name, 'agent-space-webdav', '--build-id')
        assert actual == args.expected_helper
        binary_names = release['binaries']
        hashes = command('exec', name, 'sha256sum', *['/usr/local/bin/'+binary for binary in binary_names])
        actual_hashes = {line.split()[1].rsplit('/', 1)[1]: line.split()[0] for line in hashes.splitlines()}
        assert actual_hashes == binary_names
        token_file = args.output/'management-token.private'
        token_file.write_text(management_token, encoding='utf-8')
        token_file.chmod(0o600)
        media = args.output/'media'
        media.mkdir()
        from PIL import Image
        Image.new('RGBA', (320, 80), (240, 10, 20, 90)).save(media/'image.png')
        Image.new('RGBA', (320, 80), (10, 20, 240, 90)).save(media/'changed.png')
        subprocess.run(['/usr/bin/ffmpeg', '-v', 'error', '-f', 'lavfi', '-i', 'color=red:s=320x180:r=10:d=1',
                        '-c:v', 'libx264', '-threads', '1', '-pix_fmt', 'yuv420p', str(media/'video.mp4')],
                       check=True, timeout=30)
        subprocess.run(['/usr/bin/ffmpeg', '-v', 'error', '-f', 'lavfi', '-i', 'color=red:s=320x180:r=10:d=1',
                        '-c:v', 'libvpx', '-threads', '1', '-pix_fmt', 'yuv420p', str(media/'video.webm')],
                       check=True, timeout=30)
        report.update(ready=True, container=name, volume=volume, release=release, helper=actual,
                      fixtureOwnerSHA256=hashlib.sha256((args.source/'docker/verify-previews.py').read_bytes()).hexdigest(),
                      adminOrigin='http://127.0.0.1:'+str(ports[0]), davOrigin='http://127.0.0.1:'+str(ports[1]))
        print(json.dumps({'ready': True, 'adminOrigin': report['adminOrigin'], 'mcpOrigin': report['adminOrigin'],
                          'davOrigin': report['davOrigin'], 'managementTokenFile': str(token_file),
                          'container': name, 'owner': name}), flush=True)
        for line in sys.stdin:
            if json.loads(line).get('action') == 'stop': break
    finally:
        if owner is not None:
            if command and name in owner.containers:
                try: (args.output/'agent-space.log').write_text(command('logs', name), encoding='utf-8')
                except Exception: pass
            report['cleanupErrors'] = owner.cleanup()
            report['containers'] = owner.containers
            report['volumes'] = owner.volumes
            report['cleanupPassed'] = not report['cleanupErrors']
        else:
            report['cleanupPassed'] = True
        (args.output/'management-token.private').unlink(missing_ok=True)
        (args.output/'fixture.json').write_text(json.dumps(report, indent=2)+'\n', encoding='utf-8')
        if not report['cleanupPassed']: raise RuntimeError('fixture cleanup failed; inspect fixture.json')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--image', required=True)
    parser.add_argument('--expected-source', required=True)
    parser.add_argument('--expected-helper', required=True)
    parser.add_argument('--expected-workspace')
    args = parser.parse_args()
    def terminate(*_): raise SystemExit(130)
    signal.signal(signal.SIGTERM, terminate)
    run(args)


if __name__ == '__main__': main()
