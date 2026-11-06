"""Isolated real panel CRUD, TCP/UDP forwarding, routing and two-hop chain regression."""
import os,json,socket,sqlite3,subprocess,tempfile,time,uuid,signal,shutil,urllib.request,urllib.parse,http.cookiejar,threading,http.server,struct,concurrent.futures
from pathlib import Path
os.umask(0o077)
def port():
 with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
TEST_UID = 65534 if os.geteuid()==0 else os.getuid()
TEST_GID = 65534 if os.geteuid()==0 else os.getgid()
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
echo=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);echo.bind(('127.0.0.1',0));echo.settimeout(.5);running=True
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
 template={'log':{'loglevel':'none','access':'none','error':'none'},'api':{'tag':'api','services':['HandlerService','StatsService','RoutingService']},'stats':{},'inbounds':[{'tag':'api','listen':'127.0.0.1','port':ap,'protocol':'tunnel','settings':{'address':'127.0.0.1'}}],'outbounds':[{'tag':'direct','protocol':'freedom','settings':{'finalRules':[{'action':'allow','ip':['127.0.0.0/8']}]}}],'routing':{'rules':[{'type':'field','inboundTag':['api'],'outboundTag':'api'}]},'policy':{'levels':{'0':{'statsUserUplink':True,'statsUserDownlink':True}},'system':{'statsInboundUplink':True,'statsInboundDownlink':True}}}
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

 import base64,copy
 processes=[]
 def launch(cfg,name):
  path=root/(name+'.json');path.write_text(json.dumps(cfg));os.chown(path,TEST_UID,TEST_GID)
  log=(root/(name+'.log')).open('w')
  proc=subprocess.Popen([str(root/'bin/xray-linux-amd64'),'run','-config',str(path)],cwd=root,env=env,preexec_fn=unprivileged,stdout=log,stderr=log)
  processes.append(proc)
  for item in cfg.get('inbounds',[]):
   if item.get('protocol')!='hysteria':waitport(proc,item['port'])
  return proc
 def client(outbound,name):
  cp=port();proc=launch({'log':{'loglevel':'none'},'inbounds':[{'listen':'127.0.0.1','port':cp,'protocol':'socks','settings':{'udp':True,'ip':'127.0.0.1'}}],'outbounds':[outbound]},name)
  return proc,cp
 def add(protocol,settings,stream=None):
  sp=port();form={'listen':'127.0.0.1','port':sp,'protocol':protocol,'enable':'true','remark':'matrix-'+protocol,'settings':json.dumps(settings),'streamSettings':json.dumps(stream or {'network':'tcp','security':'none'}),'sniffing':'{}'}
  result=api('inbounds/add',form);return result,form
 def apply(cfg):
  result=request('/dui/xray/update',{'xraySetting':json.dumps(cfg)});assert result['success'],result.get('msg')
  api('server/restartXrayService',{});waitport(panel,ap)
 def probe(cp,udp=True,expected=True):
  http_probe(cp,expected=expected)
  if udp:
   if expected:udp_probe(cp)
   else:
    try:udp_probe(cp)
    except (TimeoutError,socket.timeout,ConnectionError,OSError):pass
    else:raise AssertionError('blocked UDP bypassed routing')
 def relay(name):
  rp,ra=port(),port()
  cfg={'log':{'loglevel':'none'},'api':{'tag':'api','services':['StatsService']},'stats':{},
   'policy':{'system':{'statsInboundUplink':True,'statsInboundDownlink':True}},
   'inbounds':[{'tag':name,'listen':'127.0.0.1','port':rp,'protocol':'socks','settings':{'udp':True,'ip':'127.0.0.1'}},
    {'tag':'api','listen':'127.0.0.1','port':ra,'protocol':'tunnel','settings':{'address':'127.0.0.1'}}],
   'outbounds':[{'protocol':'freedom','settings':{'finalRules':[{'action':'allow','ip':['127.0.0.0/8']}]}}],
   'routing':{'rules':[{'type':'field','inboundTag':['api'],'outboundTag':'api'}]}}
  p=launch(cfg,name);return p,rp,ra,cfg
 def counter(ra,name):
  raw=subprocess.check_output([str(root/'bin/xray-linux-amd64'),'api','statsquery','--server=127.0.0.1:'+str(ra),'-pattern=inbound>>>'+name],stderr=subprocess.DEVNULL,text=True,timeout=10)
  return sum(int(x.get('value',0)) for x in json.loads(raw).get('stat',[]))
 try:
  panel=start_panel();login();cases=[]
  for protocol in ('vmess','vless','trojan','ss2022-128','ss2022-256','ss-aes128'):
   email='matrix-'+protocol;uid=str(uuid.uuid4());secret=uuid.uuid4().hex;stream={'network':'tcp','security':'none'}
   if protocol in ('vmess','vless'):
    setting={'clients':[{'id':uid,'email':email,'enable':True}]}
    if protocol=='vless':setting['decryption']='none'
    users=[{'id':uid,'encryption':'none'}] if protocol=='vless' else [{'id':uid,'security':'auto','alterId':0}]
    out={'protocol':protocol,'settings':{'vnext':[{'address':'127.0.0.1','port':0,'users':users}]},'streamSettings':stream}
   elif protocol=='trojan':
    setting={'clients':[{'password':secret,'email':email,'enable':True}]}
    stream={'network':'tcp','security':'tls','tlsSettings':{'certificates':[{'certificateFile':str(root/'cert.pem'),'keyFile':str(root/'key.pem')}]}}
    out={'protocol':'trojan','settings':{'servers':[{'address':'127.0.0.1','port':0,'password':secret}]},'streamSettings':{'network':'tcp','security':'tls','tlsSettings':{'serverName':'localhost','allowInsecure':True}}}
   else:
    size=32 if protocol=='ss2022-256' else 16
    method=('2022-blake3-aes-'+str(size*8)+'-gcm') if '2022' in protocol else 'aes-128-gcm'
    serverkey=base64.b64encode(os.urandom(size)).decode()
    userkey=base64.b64encode(os.urandom(size)).decode()
    setting={'method':method,'password':serverkey,'network':'tcp,udp','clients':[{'password':userkey,'email':email,'enable':True}]}
    if '2022' not in protocol:setting['clients'][0]['method']=method
    proxy_password=serverkey+':'+userkey if '2022' in protocol else userkey
    out={'protocol':'shadowsocks','settings':{'servers':[{'address':'127.0.0.1','port':0,'method':method,'password':proxy_password}]}}
   row,form=add('shadowsocks' if protocol.startswith('ss') else protocol,setting,stream)
   out['settings']['vnext' if protocol in ('vless','vmess') else 'servers'][0]['port']=row['port']
   proc,cp=client(out,protocol);probe(cp)
   saved_client=json.loads(api('inbounds/get/'+str(row['id']))['settings'])['clients'][0]
   if protocol=='ss-aes128':assert saved_client['method']=='aes-128-gcm','saved SS user cipher was discarded'
   client_id=uid if protocol in ('vmess','vless') else (secret if protocol=='trojan' else email)
   for enabled in (False,True):
    saved_client['enable']=enabled
    api('inbounds/updateClient/'+urllib.parse.quote(client_id,safe=''),{'id':row['id'],'settings':json.dumps({'clients':[saved_client]})})
    probe(cp,enabled,enabled)
   if protocol=='ss-aes128':
    second={'method':'chacha20-poly1305','password':uuid.uuid4().hex,'email':'ss-second','enable':True,'speedLimitMbps':3}
    api('inbounds/addClient',{'id':row['id'],'settings':json.dumps({'clients':[second]})})
    second_out=copy.deepcopy(out);second_out['settings']['servers'][0].update(method=second['method'],password=second['password'])
    second_proc,second_cp=client(second_out,'ss-second');probe(second_cp)
    second['email']='ss-second-renamed'
    api('inbounds/updateClient/ss-second',{'id':row['id'],'settings':json.dumps({'clients':[second]})});probe(second_cp)
    saved=json.loads(api('inbounds/get/'+str(row['id']))['settings'])['clients']
    assert next(c for c in saved if c['email']==second['email'])['method']==second['method']
    api('server/restartXrayService',{});waitport(panel,ap);probe(cp);probe(second_cp)
    api('inbounds/'+str(row['id'])+'/delClient/'+second['email'],{});probe(second_cp,False,False)
    print(json.dumps({'SS_per_user_cipher_create_edit_restart_delete':True}),flush=True)
   changed=dict(form);changed['enable']='false';api('inbounds/update/'+str(row['id']),changed);probe(cp,False,False)
   changed['enable']='true';api('inbounds/update/'+str(row['id']),changed);probe(cp)
   cases.append({'name':protocol,'row':row,'form':form,'out':out,'cp':cp,'proc':proc,'email':email,'udp':True})
   print(json.dumps({'protocol':protocol,'create_TCP_UDP_disable_enable':True}),flush=True)
  key=base64.b64encode(os.urandom(32)).decode();method='2022-blake3-chacha20-poly1305'
  row,form=add('shadowsocks',{'method':method,'password':key,'network':'tcp,udp','clients':[]})
  out={'protocol':'shadowsocks','settings':{'servers':[{'address':'127.0.0.1','port':row['port'],'method':method,'password':key}]}}
  proc,cp=client(out,'ss2022-chacha-single');probe(cp)
  cases.append({'name':'ss2022-chacha-single','row':row,'form':form,'out':out,'cp':cp,'proc':proc,'email':None,'udp':True})
  print(json.dumps({'protocol':'ss2022-chacha-single','create_TCP_UDP':True}),flush=True)
  for protocol in ('socks','http'):
   setting={'auth':'password','accounts':[{'user':'test','pass':'testpass'}]}
   if protocol=='socks':setting.update(udp=True,ip='127.0.0.1')
   row,form=add(protocol,setting)
   out={'protocol':protocol,'settings':{'servers':[{'address':'127.0.0.1','port':row['port'],'users':[{'user':'test','pass':'testpass'}]}]}}
   proc,cp=client(out,protocol);probe(cp,protocol=='socks')
   http_probe(row['port'],scheme='socks5h' if protocol=='socks' else 'http',expected=False)
   cases.append({'name':protocol,'row':row,'form':form,'out':out,'cp':cp,'proc':proc,'email':None,'udp':protocol=='socks'})
   print(json.dumps({'protocol':protocol,'create_TCP':True,'UDP':protocol=='socks','wrong_auth_rejected':True}),flush=True)
  for network,target in [('tcp',httpd.server_port),('udp',echo.getsockname()[1])]:
   row,form=add('tunnel',{'address':'127.0.0.1','port':target,'network':network})
   if network=='tcp':
    result=urllib.request.urlopen('http://127.0.0.1:'+str(row['port'])+'/',timeout=4).read();assert result==b'dui-native-protocol-test'
   else:
    with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as c:
     c.settimeout(4);c.sendto(b'tunnel-forwarding',('127.0.0.1',row['port']));assert c.recv(100)==b'tunnel-forwarding'
   api('inbounds/del/'+str(row['id']),{})
   print(json.dumps({'tunnel_forwarding':network,'delete':True}),flush=True)
  # Two SOCKS hops: the selected outbound dials the second relay through the first.
  # Both relay counters must grow; disabling either hop must fail closed.
  r1,p1,a1,cfg1=relay('first-hop');r2,p2,a2,cfg2=relay('second-hop')
  routed=copy.deepcopy(template)
  routed['outbounds'] += [
   {'tag':'first-hop','protocol':'socks','settings':{'servers':[{'address':'127.0.0.1','port':p1}]}},
   {'tag':'chain-exit','protocol':'socks','settings':{'servers':[{'address':'127.0.0.1','port':p2}]},'streamSettings':{'sockopt':{'dialerProxy':'first-hop'}}},
   {'tag':'reject','protocol':'blackhole'}]
  for mode in ('inbound-tag','user'):
   routed['routing']['rules']=copy.deepcopy(template['routing']['rules'])
   for case in cases:
    rule={'type':'field','inboundTag':[case['row']['tag']],'outboundTag':'chain-exit'}
    if mode=='user' and case['email']:rule['user']=[case['email']]
    routed['routing']['rules'] += [rule,{'type':'field','inboundTag':[case['row']['tag']],'outboundTag':'reject'}]
   apply(routed)
   before1,before2=counter(a1,'first-hop'),counter(a2,'second-hop')
   for case in cases:
    stop(case['proc']);case['proc'],case['cp']=client(case['out'],case['name']+'-'+mode);probe(case['cp'],case['udp'])
   assert counter(a1,'first-hop')>before1 and counter(a2,'second-hop')>before2
   print(json.dumps({'panel_saved_route':mode,'two_hops_TCP_UDP':True,'relay_counters_grew':True}),flush=True)
  one=cases[1]
  rejected=copy.deepcopy(routed)
  rejected['routing']['rules'].insert(1,{'type':'field','inboundTag':[one['row']['tag']],'outboundTag':'reject'})
  apply(rejected);probe(one['cp'],True,False)
  apply(routed);probe(one['cp'])
  stop(r1);probe(one['cp'],True,False)
  r1=launch(cfg1,'first-hop-recovered');probe(one['cp'])
  stop(r2);probe(one['cp'],True,False)
  r2=launch(cfg2,'second-hop-recovered');probe(one['cp'])
  print(json.dumps({'first_matching_rule_wins':True,'each_chain_hop_failure_rejects_new_TCP_UDP':True,'recovery':True}),flush=True)
  for case in cases:
   api('inbounds/del/'+str(case['row']['id']),{});probe(case['cp'],False,False)
  assert not api('inbounds/list')
  print(json.dumps({'all_inbounds_deleted_no_listener':True}),flush=True)
 finally:
  for p in processes:stop(p)
  stop(panel,True);running=False;echo.close();httpd.shutdown()
