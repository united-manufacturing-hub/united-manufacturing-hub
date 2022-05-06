# Generating certificates for Kafka based apps

```shell
./easyrsa gen-req APP_NAME
./easyrsa sign-req client APP_NAME
cd ./pki/private
# With password
openssl pkcs8 -topk8 -inform PEM -outform PEM -in .\APP_NAME.key -out .\APP_NAME_pkc8.key
# Without password
openssl pkcs8 -topk8 -inform PEM -outform PEM -in .\APP_NAME.key -out .\APP_NAME_pkc8.key -nocrypt
```

Now use 
 - APPNAME.crt from the issued folder as sslCertificatePem
 - APPNAME_pkc8.key from the private folder as sslKeyPem

Don't forget to set sslKeyPassword to decrypt the key or save the key decrypted, using the -nocrypt parameter in openssl pkcs8 !

# Kafka broker

The broker itself needs to have SAN fields set for it's internal URL, else kowl will refuse to connect

To do this, create a file, with your DNS names inside, and reference it by using openssl instead of easyrsa
```
subjectAltName = DNS:united-manufacturing-hub-kafka-0.united-manufacturing-hub-kafka-headless.united-manufacturing-hub.svc.cluster.local, DNS:united-manufacturing-hub-kafka
```

```shell
openssl x509 -req -in .\reqs\united-manufacturing-hub-kafka.req -days 3650 -CA .\ca.crt -CAkey .\private\ca.key -CAcreateserial -out .\issued\united-manufacturing-hub-kafka.crt -extfile .\united-manufacturing-hub-kafka.ext
```