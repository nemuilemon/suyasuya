"""Run against an isolated Compose database: creates a test nap and PVT measurement.
APP_USERNAME / APP_PASSWORD are read from the environment, never printed.
"""
import http.cookiejar,json,os,urllib.request,urllib.error,uuid,datetime
base=os.getenv('TEST_URL','http://localhost:8080')
cookies=http.cookiejar.CookieJar();client=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cookies))
def call(path,data=None,method=None,expected=200):
 req=urllib.request.Request(base+'/api'+path,data=json.dumps(data).encode() if data is not None else None,method=method,headers={'Content-Type':'application/json','X-Suyasuya':'1'})
 try:r=client.open(req)
 except urllib.error.HTTPError as e:
  assert e.code==expected,(path,e.code,e.read().decode());return
 assert r.status==expected,(path,r.status);return json.load(r)
def newID():return str(uuid.uuid4())
call('/me',expected=401)
call('/login',{'username':os.environ['APP_USERNAME'],'password':os.environ['APP_PASSWORD']})
user=call('/me')['user'];assert user['is_admin']
rec={'submission_id':newID(),'status':'approved','sleep_kind':'nap','sleep_date':'2026-09-18','timezone':'Asia/Tokyo','started_at':'','ended_at':'','duration_minutes':20}
created=call('/records',rec);assert call('/records',rec)['id']==created['id']
rec.update(id=created['id'],revision=1,submission_id=newID(),duration_minutes=25)
assert call('/records',rec)['revision']==2
rec['submission_id']=newID();call('/records',rec,expected=409)
rec.update(revision=2,submission_id=newID(),status='deleted');assert call('/records',rec)['revision']==3
assert len(call('/records/'+created['id']+'/history'))==3
assert not any(r['id']==created['id'] for r in call('/records'))
now=datetime.datetime.now(datetime.timezone.utc)
pvt={'id':newID(),'started_at':(now-datetime.timedelta(seconds=60)).isoformat(),'finished_at':now.isoformat(),'timezone':'Asia/Tokyo','device':'smoke-test','input_method':'keyboard','completed':True,'trials':[{'stimulus_at_ms':1000,'response_at_ms':1200,'rt_ms':999,'false_start':True},{'stimulus_at_ms':2000,'response_at_ms':2400,'rt_ms':0,'false_start':False}]}
m=call('/pvt',pvt);assert m['median_rt_ms']==300 and m['lapses']==1 and m['false_starts']==0,m
call('/pvt',pvt);assert sum(r['id']==pvt['id'] for r in call('/pvt'))==1
settings=call('/settings');call('/settings',{'timezone':settings['timezone'],'target_bedtime':(settings['target_bedtime'] or '')[:5],'pvt_reminder_time':(settings['pvt_reminder_time'] or '')[:5]},method='PUT')
assert len(call('/prompts'))>=2
call('/jobs');call('/logout',{});call('/me',expected=401)
print('PASS: authentication, nap revisions/deletion/idempotency, settings, PVT server metrics, prompt/job reads')
