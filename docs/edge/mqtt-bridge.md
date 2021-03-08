# MQTT bridge

## Environment variables 

REMOTE_CERTIFICATE_NAME: the certificate name / client id
REMOTE_BROKER_URL: e.g. ssl://mqtt.app.industrial-analytics.net
REMOTE_SUB_TOPIC: the remote topic that should be subscribed. The bridge will automatically append a /# to the string mentioned here
REMOTE_PUB_TOPIC: the remote topic prefix where messages from the remote broker should be send to.
REMOTE_BROKER_SSL_ENABLED: true or false

LOCAL_CERTIFICATE_NAME: the certificate name / client id
LOCAL_BROKER_URL: e.g. ssl://mqtt.app.industrial-analytics.net
LOCAL_SUB_TOPIC: the local topic that should be subscribed. The bridge will automatically append a /# to the string mentioned here
LOCAL_PUB_TOPIC: the local topic prefix where messages from the remote broker should be send to.
LOCAL_BROKER_SSL_ENABLED: true or false

BRIDGE_ONE_WAY: if true it sends the messages only from local broker to remote broker (not the other way around) 

Note regarding topics:
The bridge will append /# to LOCAL_SUB_TOPIC and subscribe to it. All messages will then be send to the remote broker. The topic on the remote broker is defined by:
1. First stripping LOCAL_SUB_TOPIC from the topic
2. and then replacing it with REMOTE_PUB_TOPIC
