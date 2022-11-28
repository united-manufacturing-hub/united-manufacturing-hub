param (
    [string]$servicename = "test-client",
    [string]$hivepass = $( Read-Host "Input password, please" )
)

Write-Host "⚙ Generating key for $servicename"
openssl req -new -x509 -newkey rsa:4096 -keyout "pki/$($servicename)-key.pem" -out "pki/$($servicename)-cert.pem" -nodes -days 3600 -subj "/CN=$($servicename)"

Write-Host "🔓 Exporting client certificate"
openssl x509 -outform der -in "pki/$($servicename)-cert.pem" -out "pki/$($servicename).crt"

Write-Host "🔓 Importing client certificate to JKS keystore"
keytool -import -file "pki/$($servicename).crt" -alias "$($servicename)" -keystore hivemq-trust-store.jks -storepass $hivepass