import json,os,socket,sqlite3,subprocess,threading,time,uuid
from pathlib import Path
from fixture import Fixture,freeport
results=[]
echo=socket.socket();echo.bind(('127.0.0.1',0));echo.listen();target=echo.getsockname()[1]
def serve():
 while True:
  try:c,_=echo.accept()
  except OSError:return
  try:
   c.settimeout(3);data=c.recv(256);c.sendall(data)
  except OSError:pass
  finally:c.close()
threading.Thread(target=serve,daemon=True).start()
def vless(port,user,domain=None):
 try:
  with socket.create_connection(('127.0.0.1',port),3) as s:
   s.settimeout(3);addr=b'\x01'+socket.inet_aton('127.0.0.1') if domain is None else b'\x02'+bytes([len(domain)])+domain.encode()
   s.sendall(b'\x00'+uuid.UUID(user).bytes+b'\x00\x01'+target.to_bytes(2,'big')+addr+b'audit-route-echo')
   data=b''
   while len(data)<18:
    chunk=s.recv(256)
    if not chunk:break
    data+=chunk
   return b'audit-route-echo' in data
 except OSError:return False
with Fixture('route-port') as f:
 op=f.login();response,form=f.add(op,email='audit-route-port');row=response[1]['obj'];oldtag=row['tag'];user=json.loads(form['settings'])['clients'][0]['id'];oldport=form['port']
 f.stop()
 with sqlite3.connect(f.db) as c:
  template=json.loads(c.execute("select value from settings where key='xrayTemplateConfig'").fetchone()[0]);template['routing']['rules'].append({'type':'field','inboundTag':[oldtag],'outboundTag':'blocked'});template['log']['access']=str(f.root/'logs/access.log');c.execute("update settings set value=? where key='xrayTemplateConfig'",(json.dumps(template),))
 f.start();before=vless(oldport,user)
 form['port']=freeport();status,obj,_=f.request('/dui/api/inbounds/update/'+str(row['id']),form,opener=op);assert obj.get('success'),obj
 newrow=f.inbounds(op)[0];after=vless(form['port'],user)
 with sqlite3.connect(f.db) as c:rule=json.loads(c.execute("select value from settings where key='xrayTemplateConfig'").fetchone()[0])['routing']['rules'][-1]
 assert newrow['tag']==oldtag and before is False and after is False
 results.append(dict(test='inbound_port_route_reference',update_success=obj.get('success'),old_tag=oldtag,new_tag=newrow['tag'],stored_rule_tag=rule['inboundTag'],before_echo_allowed=before,after_echo_allowed=after))
echo.close();print(json.dumps(results,indent=2))
