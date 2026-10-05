"""Isolated verified update API regression. Never uses an installed database/config.

Set DUI_TEST_PANEL and DUI_TEST_CORE to the binaries being tested.
The temporary panel binds loopback and runs without root privileges.
"""
import http.cookiejar
import json
import os
from pathlib import Path
import shutil
import signal
import socket
import sqlite3
import subprocess
import tempfile
import time
import urllib.parse
import urllib.request
import uuid


def port():
    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
        return s.getsockname()[1]


if os.environ.get('DUI_UPDATE_SYSTEMD_TEST') != '1' or os.geteuid() != 0:
    raise SystemExit('Opt in with DUI_UPDATE_SYSTEMD_TEST=1 on a Linux systemd test host')

uid = 65534 if os.geteuid() == 0 else os.getuid()
gid = 65534 if os.geteuid() == 0 else os.getgid()


def drop_root():
    if os.geteuid() == 0:
        os.setgroups([])
        os.setgid(gid)
        os.setuid(uid)


with tempfile.TemporaryDirectory(prefix='dui-strategy-api-') as tmp:
    root = Path(tmp)
    import hashlib
    os.chown(root, uid, gid)
    for name in ('db', 'bin', 'logs'):
        (root/name).mkdir()
        os.chown(root/name, uid, gid)
    binary = root/'bin/xray-linux-amd64'
    for src, dst in [(os.environ['DUI_TEST_PANEL'], root/'x-ui'), (os.environ['DUI_TEST_CORE'], binary)]:
        shutil.copy2(src, dst)
        dst.chmod(0o755)
    env = os.environ.copy()
    env.update(XUI_DB_FOLDER=str(root/'db'), XUI_BIN_FOLDER=str(root/'bin'),
               XUI_LOG_FOLDER=str(root/'logs'), XUI_LOG_LEVEL='error')
    panel_port, api_port = port(), port()
    password = uuid.uuid4().hex
    result = subprocess.run([str(root/'x-ui'), 'setting', '-username', 'cw-test', '-password', password,
                             '-port', str(panel_port), '-webBasePath', '/', '-listenIP', '127.0.0.1'],
                            cwd=root, env=env, preexec_fn=drop_root, capture_output=True, timeout=30)
    assert result.returncode == 0, 'isolated database initialization failed'
    template = {
        'log': {'loglevel': 'none', 'access': 'none', 'error': 'none'},
        'api': {'tag': 'api', 'services': ['HandlerService', 'StatsService']}, 'stats': {},
        'inbounds': [{'tag': 'api', 'listen': '127.0.0.1', 'port': api_port, 'protocol': 'tunnel',
                      'settings': {'address': '127.0.0.1'}}],
        'outbounds': [{'tag': 'direct', 'protocol': 'freedom'}, {'tag': 'api', 'protocol': 'blackhole'}],
        'routing': {'rules': [{'type': 'field', 'inboundTag': ['api'], 'outboundTag': 'api'}]},
        'policy': {'levels': {'0': {'bufferSize': 64}}, 'system': {'statsInboundUplink': True}}}
    missing_config = os.environ.get('DUI_TEST_MISSING_CONFIG') == '1'
    if missing_config:
        template['outbounds'].append({'tag':'plain-vless-compat','protocol':'vless','settings':{'vnext':[{'address':'8.8.8.8','port':443,'users':[{'id':str(uuid.uuid4()),'encryption':'none'}]}]},'streamSettings':{'network':'tcp','security':'none'}})
    db = root/'db/x-ui.db'
    with sqlite3.connect(db) as conn:
        conn.execute('INSERT INTO settings(key,value) VALUES(?,?)', ('xrayTemplateConfig', json.dumps(template)))

    # Isolated unit, loopback ports and fresh DB. The wrapper only maps x-ui.service
    # to this test unit. The installed business service is never addressed.
    unit='dui-update-test-'+uuid.uuid4().hex[:10]
    root.joinpath('shim').mkdir()
    shim=root/'shim/systemctl'
    shim.write_text('#!/usr/bin/python3\nimport os,sys\na=[os.environ["DUI_TEST_UNIT"] if v=="x-ui.service" else v for v in sys.argv[1:]]\nos.execv("/usr/bin/systemctl",["systemctl"]+a)\n')
    shim.chmod(0o755)
    unit_path=Path('/run/systemd/system')/(unit+'.service')
    lines=['[Unit]','Description=DUI isolated updater test','[Service]','Type=simple','WorkingDirectory='+str(root),'ExecStart='+str(root/'x-ui')]
    for k in ('XUI_DB_FOLDER','XUI_BIN_FOLDER','XUI_LOG_FOLDER','XUI_LOG_LEVEL'):
        lines.append('Environment='+k+'='+env[k])
    unit_path.write_text(chr(10).join(lines)+chr(10))
    subprocess.run(['systemctl','daemon-reload'],check=True,timeout=20)
    subprocess.run(['systemctl','start',unit],check=True,timeout=20)
    state=root/'db/updates';state.mkdir(mode=0o700)
    try:
        for _ in range(100):
            try:
                with socket.create_connection(('127.0.0.1',panel_port),timeout=.1):break
            except OSError:time.sleep(.1)
        worker=root/'worker';shutil.copy2(root/'x-ui',worker)
        worker_env=env.copy();worker_env['PATH']=str(root/'shim')+':'+env['PATH'];worker_env['DUI_TEST_UNIT']=unit+'.service'
        before=hashlib.sha256((root/'x-ui').read_bytes()).hexdigest()
        reports=[]
        for kind,version,expected in [('core','vx-26.7','complete'),('panel','v26.9.46','rolled_back')]:
            ident=uuid.uuid4().hex
            stage=root/('stage-'+ident);stage.mkdir(mode=0o700)
            meta={'id':ident,'kind':kind,'version':version,'phase':'queued','Root':str(state),'Dir':str(stage),
              'Target':str(binary if kind=='core' else root/'x-ui'),'Panel':str(root/'x-ui'),'Core':str(binary),
              'DB':str(db),'Config':str(root/'bin/config.json'),'WorkDir':str(root),'CoreRunning':True,
              'Unit':'unused-test-worker','Worker':str(stage/'worker-copy')}
            meta['PreviousCoreRunning'] = not (missing_config and kind == 'core')
            if missing_config and kind == 'core':
                assert not (root/'bin/config.json').exists(), 'missing config scenario not reproduced'
                meta['ValidationConfig'] = template
            (state/'job.json').write_text(json.dumps(meta))
            log=(stage/'worker.log').open('w')
            proc=subprocess.Popen([str(worker),'update-worker',str(state),ident],cwd=root,env=worker_env,stdout=log,stderr=log)
            seen=set();deadline=time.monotonic()+240
            while proc.poll() is None and time.monotonic()<deadline:
                try:
                    status=json.loads((state/'job.json').read_text())
                    phase=status.get('phase')
                    if phase not in seen:
                        seen.add(phase);print(json.dumps({'kind':kind,'phase':phase}),flush=True)
                except Exception:pass
                time.sleep(.5)
            if proc.poll() is None:proc.kill();raise AssertionError('isolated worker timed out')
            log.close()
            result=json.loads((state/'job.json').read_text())
            assert result['phase']==expected,{'phase':result['phase'],'error':result.get('error')}
            assert subprocess.check_output(['systemctl','is-active',unit],text=True).strip()=='active'
            assert hashlib.sha256((root/'x-ui').read_bytes()).hexdigest()==before
            with sqlite3.connect(db) as check:
                assert json.loads(check.execute("SELECT value FROM settings WHERE key='xrayTemplateConfig'").fetchone()[0]) == template
            reports.append({'missingConfigRecovery':missing_config and kind=='core','kind':kind,'result':result['phase'],'phases':sorted(seen),'restoredPanelHash':True})
        print(json.dumps({'realSystemdWorkerTests':reports}),flush=True)
    finally:
        subprocess.run(['systemctl','stop',unit],capture_output=True,timeout=30)
        unit_path.unlink()
        subprocess.run(['systemctl','daemon-reload'],check=True,timeout=20)
