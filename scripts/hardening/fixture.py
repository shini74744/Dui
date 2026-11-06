import os,json,socket,sqlite3,subprocess,tempfile,time,uuid,signal,shutil,urllib.request,urllib.parse,urllib.error,http.cookiejar
from pathlib import Path
PANEL=Path(os.environ['DUI_TEST_PANEL'])
CORE=Path(os.environ['DUI_TEST_CORE'])
UID=65534 if os.geteuid()==0 else os.getuid()
GID=65534 if os.geteuid()==0 else os.getgid()
def freeport():
 with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
def drop():
 if os.geteuid()==0:os.setgroups([]);os.setgid(GID);os.setuid(UID)
class Fixture:
 def __init__(self,name):
  self.tmp=tempfile.TemporaryDirectory(prefix='dui-audit-'+name+'-');self.root=Path(self.tmp.name);os.chown(self.root,UID,GID)
  for n in ('db','bin','logs'):
   p=self.root/n;p.mkdir();os.chown(p,UID,GID)
  for src,dst in ((PANEL,self.root/'x-ui'),(CORE,self.root/'bin/xray-linux-amd64')):shutil.copy2(src,dst);dst.chmod(0o755)
  self.env=dict(os.environ);self.env.update(XUI_DB_FOLDER=str(self.root/'db'),XUI_BIN_FOLDER=str(self.root/'bin'),XUI_LOG_FOLDER=str(self.root/'logs'),XUI_LOG_LEVEL='error',GOMAXPROCS='2',GOMEMLIMIT='256MiB')
  self.port=freeport();self.apiport=freeport();self.username='audit-admin';self.password=uuid.uuid4().hex
  self.base='http://127.0.0.1:'+str(self.port)+'/audit';self.db=self.root/'db/x-ui.db';self.proc=None
  self.cli(['setting','-username',self.username,'-password',self.password,'-port',str(self.port),'-webBasePath','/audit/','-listenIP','127.0.0.1'])
  template={'log':{'loglevel':'warning','access':'none','error':'none'},'api':{'tag':'api','services':['HandlerService','StatsService','RoutingService']},'stats':{},'inbounds':[{'tag':'api','listen':'127.0.0.1','port':self.apiport,'protocol':'tunnel','settings':{'address':'127.0.0.1'}}],'outbounds':[{'tag':'direct','protocol':'freedom','settings':{'finalRules':[{'action':'allow','ip':['127.0.0.0/8']}]}},{'tag':'blocked','protocol':'blackhole'},{'tag':'api','protocol':'blackhole'}],'routing':{'rules':[{'type':'field','inboundTag':['api'],'outboundTag':'api'}]},'policy':{'levels':{'0':{'statsUserUplink':True,'statsUserDownlink':True}},'system':{'statsInboundUplink':True,'statsInboundDownlink':True}}}
  with sqlite3.connect(self.db) as c:c.execute('INSERT INTO settings(key,value) VALUES(?,?)',('xrayTemplateConfig',json.dumps(template)))
  self.logpath=Path(os.environ.get('DUI_TEST_OUTPUT','/tmp'))/(name+'-panel.log');self.log=self.logpath.open('w')
  self.start()
 def cli(self,args):
  r=subprocess.run([str(self.root/'x-ui'),*args],cwd=self.root,env=self.env,preexec_fn=drop,capture_output=True,timeout=20);assert r.returncode==0,'fixture CLI failed'
 def start(self):
  self.proc=subprocess.Popen([str(self.root/'x-ui')],cwd=self.root,env=self.env,preexec_fn=drop,start_new_session=True,stdout=self.log,stderr=self.log)
  for p in (self.port,self.apiport):
   for _ in range(100):
    assert self.proc.poll() is None,'fixture panel exited'
    if self.listens(p):break
    time.sleep(.1)
   else:raise AssertionError('fixture port did not start')
 @staticmethod
 def listens(p):
  try:
   with socket.create_connection(('127.0.0.1',p),timeout=.3):return True
  except OSError:return False
 def request(self,path,data=None,opener=None,headers=None,raw=False):
  body=None if data is None else urllib.parse.urlencode(data).encode()
  req=urllib.request.Request(self.base+path,data=body,headers=dict({'X-Requested-With':'XMLHttpRequest'},**(headers or {})))
  client=opener or urllib.request.build_opener()
  try:r=client.open(req,timeout=15)
  except urllib.error.HTTPError as err:r=err
  with r:
   content=r.read();result=content if raw else json.loads(content)
   return r.status,result,dict(r.headers)
 def login(self,password=None):
  op=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
  status,obj,headers=self.request('/login',{'username':self.username,'password':password or self.password},opener=op)
  assert status==200 and obj.get('success'), 'fixture login failed'
  return op
 def inbounds(self,op):
  return self.request('/dui/api/inbounds/list',opener=op)[1]['obj']
 def add(self,op,port=None,enabled=True,email=None,settings=None,protocol='vless',stream=None):
  port=port or freeport();user=uuid.uuid4().hex;email=email or 'audit-'+user
  settings=settings or {'decryption':'none','clients':[{'id':str(uuid.uuid4()),'email':email,'enable':True}]}
  form={'listen':'127.0.0.1','port':port,'protocol':protocol,'enable':str(enabled).lower(),'remark':email,'settings':json.dumps(settings),'streamSettings':json.dumps(stream or {'network':'tcp','security':'none'}),'sniffing':'{}'}
  return self.request('/dui/api/inbounds/add',form,opener=op),form
 def stop(self):
  if self.proc and self.proc.poll() is None:
   os.killpg(self.proc.pid,signal.SIGTERM)
   try:self.proc.wait(timeout=8)
   except subprocess.TimeoutExpired:os.killpg(self.proc.pid,signal.SIGKILL);self.proc.wait()
  self.proc=None
 def close(self):
  self.stop();self.log.close();self.tmp.cleanup()
 def __enter__(self):return self
 def __exit__(self,*args):self.close()
