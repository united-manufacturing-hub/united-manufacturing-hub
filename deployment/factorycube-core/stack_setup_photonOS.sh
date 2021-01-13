#!/bin/bash

# Documentation
# Assumptions about the file system and the Linux system itself.

# Constants
DOCKER_VERSION=19.03.8
COMPOSE_VERSION=1.25.5
REPO_DIR="."
ENV_FILE="${REPO_DIR}/.env"
PERSISTENT_DATA="${REPO_DIR}/persistentData"

# Temporary variables
CURRENT_INSTALLED_VERSION=0

# Utility functions
## Check for super user rights
checkSuperUser() {
  if [[ "$EUID" -ne 0 ]]
    then echo "Please run as root"
    exit
  fi
}

## Compare versions. Taken from https://stackoverflow.com/questions/4023830/how-to-compare-two-strings-in-dot-separated-version-format-in-bash
compareInstalledVersion () {
  # Returns 0 if the versions are the same, 1 if v1 > v2 and 2 if v1 < v2
  if [[ $1 == $CURRENT_INSTALLED_VERSION ]]; then
    return 0
  fi
  local IFS=.
  local i ver1=($1) ver2=($CURRENT_INSTALLED_VERSION)
  # fill empty fields in ver1 with zeros
  for ((i=${#ver1[@]}; i<${#ver2[@]}; i++))
  do
      ver1[i]=0
  done
  for ((i=0; i<${#ver1[@]}; i++))
  do
      if [[ -z ${ver2[i]} ]]; then
          # fill empty fields in ver2 with zeros
          ver2[i]=0
      fi
      if ((10#${ver1[i]} > 10#${ver2[i]}))
      then
          return 1
      fi
      if ((10#${ver1[i]} < 10#${ver2[i]}))
      then
          return 2
      fi
  done
  return 0
}

## Check installed versions
checkInstallOlderVersion() {
  # $1 should be the command to check
  # $2 should be the version option
  # $3 should be the position of the version in "$1 $2"'s output
  # $4 should be the version to compare to
  #
  # Returns 0 if the installed version is equal or newer.
  # Returns 1 if the installed version is older.
  # Returns 2 if the command was not found.

  # Check if the command exists
  hash $1
  if [[ $? -ne 0 ]]; then
    return 2
  fi

  # Check if the current version is equal or greater
  extractVersion "${1} ${2}" $3

  if [[ $? -eq 0 ]]; then
    compareInstalledVersion $4
    # If the local version is equal or greater than the required, do not install.
    if [[ $? -eq 0 || $? -eq 1 ]]; then
      return 0
    fi

    # Return 1 if the installed version is older
    return 1
  fi
}

## Version extraction
extractVersion() {
  # Takes as parameters the command to extract the version from and the
  # position of the version number.
  #
  # Saves the extracted version in $CURRENT_INSTALLED_VERSION and returns 0.

  local command=$1
  local pos=$2

  # Execute the command and save the string
  local version=`$command`

  # Extract and return the version number
  IFS=' '
  read -ra VER <<< $version

  # Another run to remove possible commas
  IFS=','
  read -ra CURRENT_INSTALLED_VERSION <<< ${VER[$pos]}
  return 0
}

## Infoprinter
printFoundAndCompatible() {
  echo "Installed ${1} version is ${CURRENT_INSTALLED_VERSION}, which is equal or greaten than the requirement. Skipping installation."
}

### Docker compose
installDockerCompose() {
  echo "Checking for docker compose installation"
  checkInstallOlderVersion docker-compose --version 2 $COMPOSE_VERSION
  local check_result=$?

  if [[ $check_result -eq 0 ]]; then
    # Return 0 if the current version is equal or older than the required one
    printFoundAndCompatible "Docker Compose"
    return 0
  fi

  # Install or update docker-compose
  echo "Installing docker-compose"
  curl -L "https://github.com/docker/compose/releases/download/1.27.4/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
  chmod +x /usr/local/bin/docker-compose
  docker-compose --version
  [[ $? -eq 0 ]] && echo "Installation of docker-compose successful" || echo "Installation of docker-compose failed"
}

### Install all requirements
installRequirements() {
  echo "Installing requirements. This version of the stack was tested with the following versions:"
  echo "docker 19.03.8, docker-compose 1.25.5, git 2.17.1. The currect functioanlity of the stack"
  echo "is not guaranteed with other versions."
  installDockerCompose
  # required for later
  tdnf install unzip
}

# File system preparation
## Copy the default.env to .env and replace serial number placeholders
prepareEnvFile() {
  # Create environment variable file
  if [[ -f $ENV_FILE ]]; then
    echo "${ENV_FILE} file found, skipping creation."
  else
    echo "Creating default environment file in ${ENV_FILE}"
    cp "${REPO_DIR}/default.env" "${ENV_FILE}"
    
    # Set the transmitter ID
    sed -i 's|%IA_SERIAL_NUMBER%|'${IA_SERIAL_NUMBER}'|g' "${ENV_FILE}"

    # Set the IP range for the iolink converter by extracting the last 2 characters
    sed -i 's|%SERIAL_NUMBER_LAST_TWO%|'${IA_SERIAL_NUMBER: -2}'|g' "${ENV_FILE}"
  fi
}
## Prepare the file system for each image. At the moment, no setup for the
## IoLinkconverter, Telegraf, influx, nodered or nginx is required.
### Mosquitto
setupMosquitto() {
  echo "Setting up the MQTT broker"
  local mosquitto_dir="${PERSISTENT_DATA}/mosquitto"
  
  # Create directory for the bridge certificate
  local bridge_certificate_dir="${mosquitto_dir}/SSL_certs/bridge"
  if [[ ! -d "${bridge_certificate_dir}" ]]; then
    mkdir $bridge_certificate_dir
  fi

  # Extract certificate files. Assuming the compressed file is called $IA_SERIAL_NUMBER.zip
  # This assumes that the files will be extracted as intermediate_CA.pem
  # $IA_SERIAL_NUMBER/$IA_SERIAL_NUMBER.pem and $IA_SERIAL_NUMBER/$IA_SERIAL_NUMBER-privkey.pem
  local ca_file="${bridge_certificate_dir}/intermediate_CA.pem"
  local cert_file="${bridge_certificate_dir}/${IA_SERIAL_NUMBER}/${IA_SERIAL_NUMBER}.pem"
  local key_file="${bridge_certificate_dir}/${IA_SERIAL_NUMBER}/${IA_SERIAL_NUMBER}-privkey.pem"
  if [[ ! -f "${ca_file}" ]] || [[ ! -f "${cert_file}" ]] || [[ ! -f "${key_file}" ]]; then
    echo "Unzipping certificate files to ${bridge_certificate_dir}"
    unzip -o ${IA_SERIAL_NUMBER}.zip -d $bridge_certificate_dir
  fi

  # Modify mosquitto's conf file
  sed -i 's|%IA_SERIAL_NUMBER%|'${IA_SERIAL_NUMBER}'|g' ${mosquitto_dir}/config/mosquitto.conf
}

### Grafana
setupGrafana() {
  echo "Preparing file system permissions for Grafana. For more information checkout: https://grafana.com/docs/grafana/latest/installation/docker/#user-id-changes"
  chown -R 472:472  "${PERSISTENT_DATA}/grafana/"
}

## Prepare everyhing
setupDockerImages(){
  prepareEnvFile
  setupMosquitto
  setupGrafana
}

# Setup and run the stack
setupStack() {
  installRequirements
  setupDockerImages
}

runStack() {
  #$1 should be either "4.0" or "3.0"
  echo "Login into ia:'s docker registry"
  docker login iaproduction.azurecr.io --username c558a32f-f71a-4bf6-a034-85814c2ebac3 --password 86c9c6ed-1635-49aa-baba-24cd70091844

  if [[ "$1" == "3.0" ]]; then
    echo "Running stack version 3.0"
    docker-compose -f docker-compose.arm.yml up -d
  else
    echo "Running stack version 4.0"
    systemctl start docker
    docker-compose up -d
  fi

  # Suggest a reboot to make everything is fine
  if [[ $? -eq 0 ]]; then
    echo 'Setup done and containers running. Please'
    echo 'reboot the system and make sure everything runs.'
    echo 'You still have to setup influx and grafana. See'
    echo 'the wiki for more information.'
  else
    echo 'Container stack could not be run. See the docker'
    echo 'logs to find the cause of the problem.'
  fi
}

# Main
if [[ "$1" != "" ]]; then
  # Check super user rights
  checkSuperUser

  # Exporting required environment variables and establish local, working variables
  export IA_SERIAL_NUMBER=$1

  setupStack
  runStack $2
else
	echo "Please provide the factorycube's serial number for the installation".
	echo "USAGE: sudo ./stack_setup.sh SERIAL_NUMBER [STACK_VERSION]"
  echo "SERIAL_NUMBER     - The cube's serial number. E.g. 2020-0000"
  echo "STACK_VERSION     - If given, should be either \"4.0\" or \"3.0\"."
  echo "                    If not given, stack version 4.0 will be run."
fi

