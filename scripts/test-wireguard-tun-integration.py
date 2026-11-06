"""Privileged panel WG/TUN data-plane tests, confined to a temporary network namespace."""
import os,json,socket,sqlite3,subprocess,tempfile,time,uuid,signal,shutil,urllib.request,urllib.parse,http.cookiejar,threading,http.server,struct,concurrent.futures
from pathlib import Path
os.umask(0o077)
import sys
if '--isolated' not in sys.argv:raise SystemExit(subprocess.call(['unshare','--net',sys.executable,__file__,'--isolated']))
assert os.readlink('/proc/self/ns/net') != os.readlink('/proc/1/ns/net')
subprocess.run(['ip','link','set','lo','up'],check=True)
subprocess.run(['ip','link','add','duiwgtest0','type','dummy'],check=True)
subprocess.run(['ip','addr','add','198.18.0.1/32','dev','duiwgtest0'],check=True)
subprocess.run(['ip','link','set','duiwgtest0','up'],check=True)

def port():
 with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
TEST_UID = os.getuid()
TEST_GID = os.getgid()
def unprivileged():
 if os.geteuid()==0:os.setgroups([]);os.setgid(TEST_GID);os.setuid(TEST_UID)
class Handler(http.server.BaseHTTPRequestHandler):
 def do_GET(self):
  data=b'x'*(1024*1024) if self.path=='/rate' else b'dui-native-protocol-test';self.send_response(200);self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data)
 def do_POST(self):
  remaining=int(self.headers['Content-Length'])
  while remaining:
   chunk=self.rfile.read(min(65536,remaining))
   if not chunk:break
   remaining-=len(chunk)
  self.send_response(200 if remaining==0 else 400);self.end_headers();self.wfile.write(b'uploaded')
 def log_message(self,*args):pass
httpd=http.server.ThreadingHTTPServer(('127.0.0.1',0),Handler);threading.Thread(target=httpd.serve_forever,daemon=True).start()
echo=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);echo.bind(('0.0.0.0',0));echo.settimeout(.5);running=True
def udp_echo():
 while running:
  try:data,addr=echo.recvfrom(65535);echo.sendto(data,addr)
  except socket.timeout:pass
  except OSError:
   if not running:break
   raise
threading.Thread(target=udp_echo,daemon=True).start()
with tempfile.TemporaryDirectory(prefix='dui-native-panel-test-') as tmp:
 root=Path(tmp);os.chown(root,TEST_UID,TEST_GID)
 for name in ('db','bin','logs'):
  d=root/name;d.mkdir();os.chown(d,TEST_UID,TEST_GID)
 for src,dst in [(Path(os.environ['DUI_TEST_PANEL']),root/'x-ui'),(Path(os.environ['DUI_TEST_CORE']),root/'bin/xray-linux-amd64')]:shutil.copy2(src,dst);dst.chmod(0o755)
 subprocess.run(['openssl','req','-x509','-newkey','ec','-pkeyopt','ec_paramgen_curve:prime256v1','-nodes','-days','1','-subj','/CN=localhost','-addext','subjectAltName=DNS:localhost','-keyout',str(root/'key.pem'),'-out',str(root/'cert.pem')],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 for name in ('key.pem','cert.pem'):os.chown(root/name,TEST_UID,TEST_GID)
 env=os.environ.copy();env.update(XUI_DB_FOLDER=str(root/'db'),XUI_BIN_FOLDER=str(root/'bin'),XUI_LOG_FOLDER=str(root/'logs'),XUI_LOG_LEVEL='error')
 pp=port();ap=port();password=uuid.uuid4().hex;panel=None;probe_proc=None
 def cli(args):
  p=subprocess.run([str(root/'x-ui'),*args],cwd=root,env=env,preexec_fn=unprivileged,capture_output=True,timeout=30);assert p.returncode==0,'test panel CLI failed'
 cli(['setting','-username','dui-smoke','-password',password,'-port',str(pp),'-webBasePath','/','-listenIP','127.0.0.1'])
 db=root/'db/x-ui.db'
 def wait_db(sql,args,predicate,message):
  for attempt in range(60):
   with sqlite3.connect(db) as conn:row=conn.execute(sql,args).fetchone()
   if row is not None and predicate(row):return row
   time.sleep(.5)
  raise AssertionError(message+': '+str(row))
 template={'log':{'loglevel':'none','access':'none','error':'none'},'api':{'tag':'api','services':['HandlerService','StatsService','RoutingService']},'stats':{},'inbounds':[{'tag':'api','listen':'127.0.0.1','port':ap,'protocol':'tunnel','settings':{'address':'127.0.0.1'}}],'outbounds':[{'tag':'direct','protocol':'freedom','settings':{'finalRules':[{'action':'allow'}]}}],'routing':{'rules':[{'type':'field','inboundTag':['api'],'outboundTag':'api'}]},'policy':{'levels':{'0':{'statsUserUplink':True,'statsUserDownlink':True}},'system':{'statsInboundUplink':True,'statsInboundDownlink':True}}}
 template['outbounds'].append({'tag':'api','protocol':'blackhole'})
 with sqlite3.connect(db) as conn:conn.execute('INSERT INTO settings(key,value) VALUES(?,?)',('xrayTemplateConfig',json.dumps(template)))
 def waitport(p,number):
  for _ in range(100):
   if p.poll() is not None:raise AssertionError('test process exited')
   try:
    with socket.create_connection(('127.0.0.1',number),timeout=.1):return
   except OSError:time.sleep(.1)
  raise AssertionError('test process did not listen')
 def start_panel():
  p=subprocess.Popen([str(root/'x-ui')],cwd=root,env=env,preexec_fn=unprivileged,start_new_session=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL);waitport(p,pp);waitport(p,ap);return p
 def stop(p,group=False):
  if p is None:return
  try:
   if group:os.killpg(p.pid,signal.SIGTERM)
   else:p.terminate()
  except ProcessLookupError:pass
  try:p.wait(timeout=8)
  except subprocess.TimeoutExpired:
   if group:os.killpg(p.pid,signal.SIGKILL)
   else:p.kill()
   p.wait()
 def login():
  global opener
  opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()));assert request('/login',{'username':'dui-smoke','password':password})['success']
 def request(path,data=None):
  body=None if data is None else urllib.parse.urlencode(data).encode()
  with opener.open('http://127.0.0.1:'+str(pp)+path,body,timeout=30) as r:return json.load(r)
 def api(path,data=None):
  r=request('/dui/api/'+path,data);assert r.get('success'), 'isolated API failed: '+path+' '+str(r.get('msg',''));return r.get('obj')
 def coreapi(args):
  r=subprocess.run([str(root/'bin/xray-linux-amd64'),'api',args[0],'--server=127.0.0.1:'+str(ap),*args[1:]],capture_output=True,text=True,timeout=12);assert r.returncode==0,'test core API failed: '+r.stderr+r.stdout;return r.stdout
 def http_probe(cp,auth=None,scheme='socks5h',expected=True):
  proxy=scheme+'://'+(('test:testpass@') if auth else '')+'127.0.0.1:'+str(cp)
  r=subprocess.run(['curl','--noproxy','','-sS','--max-time','3','--proxy',proxy,'http://127.0.0.1:'+str(httpd.server_port)+'/'],capture_output=True,timeout=5)
  assert (r.returncode==0 and r.stdout==b'dui-native-protocol-test')==expected,'HTTP proxy result mismatch: '+scheme+' expected='+str(expected)
 def udp_probe(cp):
  with socket.create_connection(('127.0.0.1',cp),timeout=3) as control:
   control.sendall(b'\x05\x01\x00');assert control.recv(2)==b'\x05\x00'
   control.sendall(b'\x05\x03\x00\x01'+socket.inet_aton('127.0.0.1')+b'\x00\x00');reply=control.recv(64);assert reply[:2]==b'\x05\x00'
   relay=(socket.inet_ntoa(reply[4:8]),int.from_bytes(reply[8:10],'big'))
   with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as packet:
    packet.settimeout(4);payload=b'native-udp-'+uuid.uuid4().bytes
    packet.sendto(b'\x00\x00\x00\x01'+socket.inet_aton('127.0.0.1')+echo.getsockname()[1].to_bytes(2,'big')+payload,relay)
    response,_=packet.recvfrom(1024);assert response[10:]==payload,'UDP echo mismatch'
 def hy_client(sp,auth):
  cp=port();cfg={'log':{'loglevel':'none'},'inbounds':[{'listen':'127.0.0.1','port':cp,'protocol':'socks','settings':{'auth':'noauth','udp':True,'ip':'127.0.0.1'}}],'outbounds':[{'protocol':'hysteria','settings':{'version':2,'address':'127.0.0.1','port':sp},'streamSettings':{'network':'hysteria','security':'tls','hysteriaSettings':{'version':2,'auth':auth},'tlsSettings':{'serverName':'localhost','allowInsecure':True,'alpn':['h3']}}}]}
  path=root/'probe.json';path.write_text(json.dumps(cfg));os.chown(path,TEST_UID,TEST_GID)
  p=subprocess.Popen([str(root/'bin/xray-linux-amd64'),'run','-config',str(path)],cwd=root,env=env,preexec_fn=unprivileged,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL);waitport(p,cp);return p,cp

 import base64,select,copy
 awgc=None
 def wg_command(**kw):
  awgc.stdin.write(json.dumps(kw)+'\n');awgc.stdin.flush()
  ready,_,_=select.select([awgc.stdout],[],[],10);assert ready,'WireGuard client timed out'
  return json.loads(awgc.stdout.readline())
 def apply(cfg):
  result=request('/dui/xray/update',{'xraySetting':json.dumps(cfg)});assert result['success'],result.get('msg')
  api('server/restartXrayService',{});waitport(panel,ap)
 def udp_plain(host,payload):
  with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as c:
   c.settimeout(4);c.sendto(payload,(host,echo.getsockname()[1]));assert c.recv(1024)==payload
 try:
  panel=start_panel();login()
  awgc=subprocess.Popen([os.environ['DUI_TEST_AWG_CLIENT']],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,text=True)
  sk=wg_command(op='keys');ck=wg_command(op='keys')
  wp=port()
  form={'listen':'127.0.0.1','port':wp,'protocol':'wireguard','enable':'true','remark':'WireGuard isolated test',
   'settings':json.dumps({'secretKey':sk['private'],'address':['10.70.0.1/24'],'mtu':1280,'noKernelTun':True,'peers':[{'publicKey':ck['public'],'allowedIPs':['10.70.0.2/32'],'email':'wg-test'}]}),'streamSettings':'{}','sniffing':'{}'}
  row=api('inbounds/add',form)
  conf='private_key='+base64.b64decode(ck['private']).hex()+'\npublic_key='+base64.b64decode(sk['public']).hex()+'\nendpoint=127.0.0.1:'+str(wp)+'\nallowed_ip=0.0.0.0/0\n'
  assert wg_command(op='setup',address='10.70.0.2',config=conf)['ok']
  # An actual TCP/UDP echo endpoint; the usermode client routes through WireGuard.
  tcp=socket.socket();tcp.bind(('198.18.0.1',0));tcp.listen()
  def echo_tcp():
   while True:
    try:c,_=tcp.accept()
    except OSError:return
    def relay(c):
     with c:
      try:
       c.settimeout(5)
       while True:
        b=c.recv(4096)
        if not b:return
        c.sendall(b)
      except OSError:pass
    threading.Thread(target=relay,args=(c,),daemon=True).start()
  threading.Thread(target=echo_tcp,daemon=True).start()
  def wg_probe(expect=True):
   for network,p in [('tcp',tcp.getsockname()[1]),('udp',echo.getsockname()[1])]:
    result=wg_command(op='probe',network=network,destination='198.18.0.1:'+str(p))
    assert result['ok']==expect,'WireGuard '+network+' '+result.get('error','')
  wg_probe()
  cfg=copy.deepcopy(template);cfg['outbounds'].append({'tag':'deny','protocol':'blackhole'})
  cfg['routing']['rules'].append({'type':'field','inboundTag':[row['tag']],'outboundTag':'deny'})
  apply(cfg);wg_probe(False)
  apply(template)
  # Reestablish client handshake after a server restart.
  assert wg_command(op='setup',address='10.70.0.2',config=conf)['ok'];wg_probe()
  api('inbounds/del/'+str(row['id']),{})
  print(json.dumps({'WireGuard_panel_create_TCP_UDP':True,'route_reject_and_recovery':True,'delete':True}),flush=True)
  tcp.close()
  # TUN only changes this temporary network namespace.
  cfg=copy.deepcopy(template)
  cfg['outbounds'].insert(0,{'tag':'local-echo','protocol':'freedom','settings':{'redirect':'127.0.0.1:'+str(httpd.server_port),'finalRules':[{'action':'allow'}]}})
  cfg['outbounds'].append({'tag':'udp-echo','protocol':'freedom','settings':{'redirect':'127.0.0.1:'+str(echo.getsockname()[1]),'finalRules':[{'action':'allow'}]}})
  cfg['routing']['rules'].append({'type':'field','network':'udp','outboundTag':'udp-echo'})
  apply(cfg)
  tf={'listen':'','port':0,'protocol':'tun','enable':'true','remark':'isolated TUN','settings':json.dumps({'name':'duimemtest0','mtu':1500,'gateway':['192.0.2.1/30'],'autoSystemRoutingTable':['198.19.0.2/32'],'autoOutboundsInterface':'lo'}),'streamSettings':'{}','sniffing':'{}'}
  tr=api('inbounds/add',tf)
  for _ in range(100):
   if subprocess.run(['ip','link','show','duimemtest0'],capture_output=True).returncode==0:break
   time.sleep(.1)
  else:raise AssertionError('TUN interface did not start')
  result=subprocess.run(['curl','--noproxy','*','-fsS','--max-time','5','http://198.19.0.2:'+str(httpd.server_port)+'/'],capture_output=True,timeout=7)
  assert result.returncode==0 and result.stdout==b'dui-native-protocol-test','TUN TCP forwarding failed'
  udp_plain('198.19.0.2',b'tun-UDP-forward')
  api('inbounds/del/'+str(tr['id']),{})
  for _ in range(100):
   if subprocess.run(['ip','link','show','duimemtest0'],capture_output=True).returncode!=0:break
   time.sleep(.1)
  else:raise AssertionError('TUN interface retained after deletion')
  assert not api('inbounds/list')
  print(json.dumps({'TUN_panel_create_TCP_UDP':True,'delete_removes_interface':True,'isolated_network_namespace':True}),flush=True)
 finally:
  stop(awgc);stop(panel,True);running=False;echo.close();httpd.shutdown()
