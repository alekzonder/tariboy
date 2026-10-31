#!/usr/bin/env python3
import json, os, sys
config = json.loads(os.environ.get('TEST_OPENCODE_CONFIG', '{}'))
def merge(a,b):
    for k,v in b.items():
        if isinstance(v,dict) and isinstance(a.get(k),dict): merge(a[k],v)
        else: a[k]=v
merge(config,json.loads(os.environ.get('OPENCODE_CONFIG_CONTENT','{}')))
if sys.argv[1:]==['debug','config']:
    if os.environ.get('TEST_IGNORE_OVERLAY'): config=json.loads(os.environ['TEST_OPENCODE_CONFIG'])
    if os.environ.get('TEST_CHANGE_FINAL_MODEL') and config.get('enabled_providers'): config['small_model']='other/small'
    print(json.dumps(config))
elif sys.argv[1:]==['models','--verbose']:
    for provider,model,sdk in [('kimi-code-plan-global','k3','@ai-sdk/openai-compatible'),('other','small','@ai-sdk/anthropic')]:
        if config.get('enabled_providers') and provider not in config['enabled_providers']: continue
        print(provider+'/'+model)
        print(json.dumps({'id':model,'providerID':provider,'api':{'id':model,'url':'https://api.kimi.ai/coding/v1' if provider.startswith('kimi') else 'https://api.anthropic.com/v1','npm':sdk},'options':{}}))
elif sys.argv[1]=='run':
    open('created','w').close()
    print(json.dumps(config))
else: sys.exit(9)
