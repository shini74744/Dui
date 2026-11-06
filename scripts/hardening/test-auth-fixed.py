import json,uuid,concurrent.futures,re,urllib.request
from fixture import Fixture
rows=[]
with Fixture('auth-fixed') as f:
 for path in ['/getDb','/getConfigJson','/status','/helperCapabilities','/swap/status','/history/load','/getNewUUID','/dui/api/server/getDb','/dui/api/inbounds/list']:
  status,_,_=f.request(path);assert status==401,(path,status)
 for path in ['/stopXrayService','/restartXrayService','/updates/check','/updates/start','/swap/apply','/openPort','/importDB','/install/subconverter']:
  status,_,_=f.request(path,{});assert status==401,(path,status)
 assert f.listens(f.apiport)
 rows.append({'anonymous_routes_rejected':17,'core_unchanged':True})
 a,b=f.login(),f.login();new=uuid.uuid4().hex;third=uuid.uuid4().hex
 status,obj,headers=f.request('/dui/setting/updateUser',{'oldUsername':f.username,'oldPassword':f.password,'newUsername':f.username,'newPassword':new},opener=a);assert obj['success'],obj
 assert f.request('/dui/api/inbounds/list',opener=a)[0]==200
 assert f.request('/dui/api/inbounds/list',opener=b)[0]==401
 assert f.request('/dui/setting/updateUser',{'oldUsername':f.username,'oldPassword':f.password,'newUsername':f.username,'newPassword':third},opener=b)[0]==401
 f.login(new)
 status,obj,_=f.request('/login',{'username':f.username,'password':f.password});assert not obj.get('success')
 rows.append({'password_change_revokes_old_sessions':True,'current_session_saved':bool(headers.get('Set-Cookie')),'new_password_login':True})
 def title(lang):
  req=urllib.request.Request(f.base+'/',headers={'Accept-Language':lang})
  html=urllib.request.urlopen(req,timeout=15).read().decode();return re.search(r'<title>(.*?)</title>',html,re.S).group(1).strip()
 en,zh=title('en-US'),title('zh-CN');assert en!=zh,(en,zh)
 langs=['en-US','zh-CN']*20
 with concurrent.futures.ThreadPoolExecutor(max_workers=6) as ex:answers=list(ex.map(title,langs))
 assert all(value==(en if lang=='en-US' else zh) for lang,value in zip(langs,answers))
 rows.append({'concurrent_HTML_languages_isolated':True,'english_title':en,'chinese_title':zh})
print(json.dumps(rows,ensure_ascii=False,indent=2))
