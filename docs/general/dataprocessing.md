# Dataprocessing in the United Manufacturing Hub / architecture

This document gives you a quick introduction into how data is processed in the United Manufacturing Hub. Furthermore, the high-level architecture is explained.

## Contents

- [Dataprocessing in the United Manufacturing Hub / architecture](#dataprocessing-in-the-united-manufacturing-hub--architecture)
  - [Contents](#contents)
  - [Architecture diagram](#architecture-diagram)
  - [Microservice approach](#microservice-approach)
  - [Services and interfaces in the UMH](#services-and-interfaces-in-the-umh)
    - [MQTT](#mqtt)
    - [REST / HTTP](#rest--http)
  - [Practical implications](#practical-implications)
    - [Edge devices](#edge-devices)
    - [Server](#server)
      - [Database access](#database-access)

## Architecture diagram

![microservice architecture](images/dataprocessing.svg)

## Microservice approach

The UMH is based on the [microservices approach](https://en.wikipedia.org/wiki/Microservices) and arranges an application as a collection of loosely coupled services. Each service is a self-contained piece of business functionality with clear interfaces.

## Services and interfaces in the UMH

A service in UMH can either be on the server or the edge device (see [general architecture](#general-architecture)). The interface between all edge and most of the server services is MQTT. The interface between Grafana (or any other BI tool) and factoryinsight is REST.

### MQTT

Out of the [wikipedia article about MQTT](https://en.wikipedia.org/wiki/MQTT):

> MQTT (originally an acronym for MQ Telemetry Transport) is an lightweight, publish-subscribe network protocol that transports messages between devices. The MQTT protocol defines two types of network entities: a message broker and a number of clients. An MQTT broker is a server that receives all messages from the clients and then routes the messages to the appropriate destination clients. An MQTT client is any device (from a micro controller up to a fully-fledged server) that runs an MQTT library and connects to an MQTT broker over a network.

The protocol MQTT specifies not the actual content of the exchanged MQTT messages. In the UMH stack we specify them further with the [UMH datamodel](mqtt.md)

### REST / HTTP

Out of the [wikipedia article about REST](https://en.wikipedia.org/wiki/Representational_state_transfer):

> Representational state transfer (REST) is a de-facto standard for a software architecture for interactive applications that typically use multiple Web services. In order to be used in a REST-based application, a Web Service needs to meet certain constraints; such a Web Service is called RESTful. A RESTful Web service is required to provide an application access to its Web resources in a textual representation and support reading and modification of them with a stateless protocol and a predefined set of operations. By being RESTfull, Web Services provide interoperability between the computer systems on the internet that provide these services.

factoryinsight provides a REST / HTTP access to all data stored in the database. [The entire API is well-documented](../server/factoryinsight/openapi/factoryinsight.yml).

## Practical implications

### Edge devices

Typically you have multiple data sources like `sensorconnect` or `barcodereader`, that are containered in a Docker container. They all send their data to the MQTT broker. You can now process the data in node-red by subscribing to the data sources via MQTT, processing the data, and then writing it back.

### Server

#### Database access

The database on the server side should never be accessed directly by a service except `mqtt-to-postgresql` and `factoryinsight`. Instead, these services should be modified to include the required functionalities.
