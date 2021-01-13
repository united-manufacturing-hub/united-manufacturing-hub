# Tutorial

# Building the stack

1. Access the certificates directory.
    ```
    $ cd ia-factorycube/persistentData/certificates/
    ```
2. Prepare the stack's environment by renaming ```default.env``` to ```.env``` and changing the value of the variables to your specific environment. ***DO NOT CHANGE*** the variables unless you know what you are doing.
3. Place the CA file for the MQTT bridge (you get it from the server admin), in the ```./ia-factorycube/persistentData/mosquitto/SSL_certs/bridge```, the corresponging client certificate files in ```./ia-factorycube/persistentData/mosquitto/SSL_certs/bridge/client_name``` and modify the mosquitto configuration file accordingly under the Bridge section.
4. Build the images with docker compose.
   ```
   $ sudo docker-compose build
   ```

The software will automatically search for new IO-link Gateways in IP_RANGE (default: 172.16.1.0/24) and will startup nodered on port 80 of the host.