# Generating certificates for Kafka based apps

```shell
./easyrsa gen-req APP_NAME
./easyrsa sign-req client APP_NAME
cd ./pki/private
openssl pkcs8 -topk8 -inform PEM -outform PEM -in .\APP_NAME.key -out .\APP_NAME_pkc8.key
```

Now use 
 - APPNAME.crt from the issued folder as sslCertificatePem
 - APPNAME_pkc8.key from the private folder as sslKeyPem

Don't forget to set sslKeyPassword to decrypt the key !