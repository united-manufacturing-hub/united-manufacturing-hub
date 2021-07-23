version: '3.7'
networks:
  mynet:
services:
  minio:
    container_name: minio
    image: minio/minio
    ports:
    - published: 9000
      target: 9000
    command: server /data
    networks:
      mynet:
        aliases:
        # For localhost access, add the following to your /etc/hosts
         127.0.0.1       my-minio-localhost-alias
        # When accessing the minio container on a server with an accessible dns, use the following
        # - ${HOSTNAME}
  # When initializing the minio container for the first time, you will need to create an initial bucket named my-bucket.
  minio-mc:
    container_name: minio-mc
    image: minio/mc
    depends_on:
    - minio
    volumes:
    - "./hello.txt:/tmp/hello.txt"
    networks:
      mynet:
  s3-client:
    container_name: s3-client
    image: amazon/aws-cli
    environment:
      AWS_ACCESS_KEY_ID: minioadmin
      AWS_SECRET_ACCESS_KEY: minioadmin
    depends_on:
    - minio
    networks:
      mynet:
      
      
      
#"""
#version: "3.2"
#services:
#  minio:
#    image: minio/minio:latest
#    entrypoint: sh
#    command: -c 'mkdir -p /export/gitlab && /usr/bin/minio server /export'
#    environment:
#          MINIO_ACCESS_KEY: "DCCAachen"
#          MINIO_SECRET_KEY: "DCCAachen2021"
#    deploy:
#      restart_policy:
#        condition: on-failure
# 

