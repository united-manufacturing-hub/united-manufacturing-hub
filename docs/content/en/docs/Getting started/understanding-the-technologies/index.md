---
title: "0. Understanding the technologies"
linkTitle: "0. Understanding the technologies"
weight: 1
description: >
  Strongly recommended. This section gives you an introduction into the used technologies. A rough understanding of these technologies is fundamental for installing and working with the system. Additionally, this article provides further learning materials for certain technologies.
---

The materials presented below are usually teached in a 2-3 h workshop session on a live production shopfloor at the Digital Capability Center Aachen. You can find the outline further below. 

## Introduction into IT / OT

The goal of this chapter is to create a common ground on IT / OT technologies and review best-practices for using IIoT technologies. The target group are people coming from IT, OT and engineering.

### Introduction

{{< imgproc introduction_1.png Fit "1280x500" >}}IIoT sits at the intersection of IT and OT.{{< /imgproc >}}

{{< imgproc introduction_2.png Fit "1280x500" >}}History: IT & OT were typically seperate silos but are currently converging to IIoT{{< /imgproc >}}

### Operational Technology (OT)

#### OT connects own set of various technologies to create highly reliable and stable machines

OT is the hardware and software to manage, monitor and control industrial operations. Its tasks range from monitoring critical assets to controlling robots on the shopfloor. It basically keeps machines and factories running and producing the required product.

##### Typical responsibilities:
- Monitoring processes to ensure best product quality
- Controlling machine parameters 
- Automation of mechanical and controlling processes
- Connecting machines and sensors for communication 
- Maintenance of machines and assets
- Certifiying machines for safety and compliance
- Retrofitting assets to increase functionality
- And many more…

##### Typical vendors for Operational Technology:
- [Schneider Electric](https://www.se.com/)
- [Rockwell Automation](https://www.rockwellautomation.com/)
- [Honeywell](https://www.honeywell.com/)
- [Mitsubishi Electric](https://mitsubishielectric.com/)
- [National Instruments](https://www.ni.com/)
- [Omron](https://omron.de/)
- [Siemens](https://new.siemens.com/de/de/produkte/automatisierung/themenfelder/simatic.html)


#### The concepts of OT are close to eletronics and with a strong focus on human and machine safety

| Concept                 | Description                                                                                                                                                                                   | Examples                                                                                                 |
|-------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------|
| Process control         | Designing a stable process which create the desired output with continuously changing inputs   External and internal factors influence the process but are not allowed to change the result​ | Controlling refrigerator based on internal temperature​                                                  |
| Sensor technology​      | Using various sensors types to measure pressure, force, temperature, velocity, etc.​  Converting sensor signals to digital outputs, interpreting their signals and generating insights​       | Light barrier counting parts on a conveyor belt​  Vibration sensor for process control in CNC machining​ |
| Automation              | Using hardware and software to automate repetitive or dangerous tasks​  Reducing reaction time and increasing speed to increase productivity​                                                 | Robot assembling smartphones ​                                                                           |
| Reliability and safety​ | Ensuring that neither humans nor machines are damaged in case of unforeseen events​  Regular checks, maintenance and certification of crucial assets​                                         | Emergency stop buttons for worker safety ​  Regular maintenance to prevent machine breakdown​            |

#### OT focuses on handling processes with highest possible safety for machines and operators


##### High importance in OT:
| Category                     | Description                                                                           |
|------------------------------|---------------------------------------------------------------------------------------|
| **Reliability & Safety**​        | Malfunction can result in extreme damages to human and property​                      |
| **Maintainability & standards**​ | Machines typically run between 20-30 years, sometimes even 50+ years​                 |
| **Certifications**​              | Legally required certifications for safety and optional certificates for reliability​ |

##### Of lesser importance:
| Category                               | Description                                                                                     |
|----------------------------------------|-------------------------------------------------------------------------------------------------|
| **User experience**​                       | The operator will be trained anyway, therefore intuitive User Interfaces (UI) are not required​ |
| **Quick development cycles, e.g., agile**​ | Can result in missing out important safety elements and damage workers or machines​             |
| **IT Security**​                           | 20+ year old machines are not designed with cyber security in mind​                             |
> **Nobody wants to build a nuclear reactor using agile "move fast, break things" principles**

#### Typical device architecture and situation for the OT

{{< imgproc ot_1.png Fit "1280x500" >}}Typical device architecture and situation for the OT. 

1 Programmable logic controller; 
2 Human Machine Interface; 
3 Supervisory Control and Data Acquisition{{< /imgproc >}}

#### Fundamentals 1: Programmable Logic Controller (PLC)

The Programmable Logic Controller (PLC) is the heart of every modern machine, which stores and runs the program. It is a PC with industrial standards and does not require a monitor, keyboard or other devices to function properly. It collects sensor data and calculates complex algorithms to control actuators.

##### Background:
- Very old machines use only relays (electric switches) to control actuators and sensors
- PLCs were introduced due to being more reliable and flexible than electrical parts
- The logic of simple switches is still very present in the OT space (programming)

##### Programming languages:
- The various suppliers like Siemens, Rockwell, Bosch etc. offer different programming languages
- PLCs can be programmed with graphical elements or with code 
- Machine vendor programs are not always openly accessible and do not allow changes (loss of warranty)

##### Communication protocols:
- PLC manufacturers have different communication protocols and functional standards which limits interoperability
- Newer protocols like Profinet or Ethernet/IP are easy to connect to an IT network (if open interface). Others like Profibus require additional hardware and implementation effort

#### Fundamentals 2: PLCs & PLC programming

{{< youtube PbAGl_mv5XI >}}

#### Fundamentals 3: Process control using PLCs

{{< youtube sFqFrmMJ-sg >}}

### Information Technology (IT)

#### IT connects millions of devices and manages their data flows

IT is the hardware and software to connect thousands of devices in a network and manage their exchange of information. The purpose is to enable data storage and its usage for business and operations. Tasks range from connecting simple telephones to managing complex global networks.

##### Typical responsibilities:
- Setting up phones, PCs, printers and other office hardware
- Monitoring devices and networks for security breaches
- Maintaining local servers
- Configuration of business systems e.g. ERP/SAP
- Updating devices to ensure IT security
- Setting up local networks and WiFi 
- Implementing business solutions like automation and 
- And many more…

##### Typical vendors:
- [Microsoft](https://www.microsoft.com/)
- [Cisco](http://www.cisco.com)
- [Apple](https://apple.com)
- [Dell](https://www.dell.com/)
- [AWS](https://aws.amazon.com)

#### The concepts of IT are focusing on digital data and networks


| Concept | Description | Examples |
|---|---|---|
| Data storage and analytics​ | Data has to be managed and stored in a manner which allows quick access and driving insights to improve business KPIs​  Terabytes of data without contextualization does not have any business value ​ | Aggregating sales data and calculating KPIs every quarter​ |
| Device Management​ | Remote device management allows the monitoring and updating of devices​  Blocking and updating devices to reduce security risks and malicious actions ​ | Updating and restarting computer remotely​ |
| Network security ​ | Policies, processes and practices like firewalls and two-factor authentication adopted to prevent cyber attacks​  Limiting risk by limiting the number of accesses and rights of users e.g. not all users are admins, users are only granted access when it is required for their work etc. ​ | Limiting internet access to specific services​ |
| Scalability​ | New software and functionality can be installed and rolled out only with a few clicks​  Update to existing solutions does not always require new hardware like in OT​ | Handing out Microsoft Office to all employees​ |

#### What is important in IT? What is not important?

##### High importance in IT:

| Category | Description |
|---|---|
| **Quick developmentcycles, e.g., agile**​ | Good user experience is more important than a perfectly designed app​ |
| **Scalability**​ | Apps need to handle millions of users at the same time (e.g., Google, Netflix)​ |
| **User experience**​ | If something is unintuitive, people tend to not use it​ |

##### Of lesser importance:

| Category | Description |
|---|---|
| **Reliability & Safety** | Hardware is redundant, if one fails another can take over Consequences of hardware failures are smaller​ |
| **Maintainability & standards** | Standards are usually best-practices and might change over time. No hard-written norms.​ |
| **Certifications** | Therefore, certifications are not legally required​ |

> **Nobody wants to build an app for years just so that the end-user removes it within 30 seconds**

#### Fundamentals 1: Networking

{{< youtube keeqnciDVOo >}}

#### Fundamentals 2: Cloud and Microservices

The term cloud refers to servers and the software running on them. These servers can be used to compute data e.g. process a customer order or simulate the weather and at the same time store it. This data can be accessed around the globe simultaneously with high-speed which enables a centralized “single source of truth”

##### Cloud products:
- Cloud providers offer their capabilities on advanced analytics and machine learning to reduce time for insights generation (Platform as a Service - PaaS)
- Storage and computational power can be booked flexibly and used freely 
- Out of the box applications running in the browser without installation

##### Microservices:
- Small stand alone blocks running only small functions
- Whenever one microservice block crashes the rest is unaffected (high stability)
- One solutions can be designed out of multiple already available microservices

##### Scalability:
- Microservice blocks can be flexibly turned on and off depending on the user requirements
- Easy scalability allows customers to only pay what they use
- Single solutions can be deployed and accesses globally without installation on each personal computer

#### Fundamentals 3: How microservices are built: Docker in 100 seconds

{{< youtube Gjnup-PuquQ >}}

#### Fundamentals 4: How to orchestrate IT stacks: Kubernetes in 100 seconds

{{< youtube PziYflu8cB8 >}}

#### Fundamentals 5: Typical network setups in production facilities

{{< imgproc it_1.png Fit "1280x500" >}}Typical network setups in production facilities{{< /imgproc >}}

### Industrial Internet of Things (IIoT)

#### Whats it's all about

##### Why is digital transformation relevant now?
Technology advancements have lowered barriers to industrial IoT to come down. The benefits of IIoT are real and sizable. 

##### How can manufacturing organizations capture value at scale?
A digital transformation in manufacturing requires an orchestrated approach across the dimensions of business, organization and technology. A holistic framework focuses on full value capture through having a return-on-investment, capability building and technical IIoT ecosystem focus

##### Value created through digital transformation
Following a digital transformation approach cases show great impact in e.g., throughput, production efficiency, gross margin, quality across various industries.

#### A full digital transformation of manufacturing needs to consider business, technology and organization

##### Business: Impact Drive Solutions
- Impact comes from a top-down prioritized portfolio of use cases to address highest value first 
- Digital transformations need to have a return-on-investment mindse

##### Organization: New way of working and dedicated approach to skills and capabilities
- Digital transformations are multiyear, company-wide journeys requiring a central transformation engine and a value capture approach 
- Innovative digital-capability-building approaches allow the rapid upskilling of thousands of employee

##### Technology: Cloud enabled platforms and ecosystem (focus of the United Manufacturing Hub)
- Form a comprehensive, secure, affordable and scalable technology infrastructure based on IoT platform architectures 
- Build and lead a focused ecosystem of technology partner

#### IIoT sits at the intersection of IT and OT

{{< imgproc introduction_1.png Fit "1280x500" >}}IIoT sits at the intersection of IT and OT.{{< /imgproc >}}

#### Architecting for scale

{{< imgproc iiot_1.jpg Fit "1280x500" >}}Architecting for scale.{{< /imgproc >}}

#### Best-practices 1: Avoid common traps in the IIoT space

*See also [Open source in Industrial IoT: an open and robust infrastructure instead of reinventing the wheel](/docs/concepts/open-source-industrial-iot/)*

##### Avoid lock-in effects
- Use open and freely accessible standards
- Use open-source whenever reasonable
- When acquiring new software, hardware or machines define contracts on data property, security plans and service levels
- Check for and request full and understandable documentation of each device and interface

##### Use both of best worlds (IT and OT)

Look into the other world to get alternative solutions and inspiration e.g.
- Grafana dashboard instead of a built in HMI
- Industrial PCs instead of Raspberry PIs
- Secure crucial connections with firewalls (e.g. pfSense)

##### Avoid quick fixes
- Use industry-wide IT/OT standards wherever applicable before developing on your own
- Invest enough time for architecture basics and the big picture to avoid rework in the near future
- Always document your project even when it seem unnecessary at the moment 

#### Best-practices 2: Protocols which allow communication of IT and OT systems

##### Coming from IT: MQTT
{{< imgproc iiot_2.png Fit "1280x500" >}}MQTT{{< /imgproc >}}
- A light weight with low-bandwidth and power requirements which is the leading standard for IoT applications 
- All devices connect to a MQTT Broker which saves and distributes information to devices which subscribed to it (similar to social network)

##### Coming from IT: REST API
{{< imgproc iiot_3.png Fit "1280x500" >}}REST API{{< /imgproc >}}
- Standard web application interface which is used on almost every website
- Can be used to request information from the web e.g. weather data or distance between two cities (Google Maps) or request actions e.g. save a file in the database

##### Coming from OT: OPC/UA
{{< imgproc iiot_4.png Fit "1280x500" >}}OPC/UA{{< /imgproc >}}
- Standard interface for automation and machine connectivity
- Highly complex protocol with wide range of capabilities but low in user friendliness

#### Best-practices 3: Reduce complexity in machine connection with tools like Node-RED

*See also [Node-RED in Industrial IoT: a growing standard](/docs/concepts/node-red-in-industrial-iot/)*

#### Best-practices 4: Connect IT and OT securely using a Demilitarized Zone (DMZ)

*See also [Why are our networks open by default and how do I protect my valuable industrial assets?](https://www.linkedin.com/pulse/why-our-networks-open-default-how-do-i-protect-my-valuable-proch/?trk=articles_directory)*

#### Architecture

*See also [Architecture](/docs/concepts/)*

#### Example projects

*See also [Examples](/docs/examples/)*

