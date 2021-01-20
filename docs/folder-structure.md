# Folder structure

This repository contains multiple folders and sub-projects:

- **/golang** contains software developed in Go, especially [factoryinsight](#factoryinsight) and [mqtt-to-postgresql](#mqtt-to-postgresql) and their corresponding tests (-environments)
- **/deployment** contains all deployment related files for the server and the factorycube, e.g. based on Kubernetes or Docker, sorted in seperate folders
- **/sensorconnect** contains [sensorconnect](#sensorconnect)
- **/grafana-plugins/factoryinsight-datasource** contains [factoryinsight-datasource](#factoryinsight-datasource)
- **/barcodereader** contains [barcodereader](#barcodereader)
- **/python-sdk** contains a template and examples to analyze data in real-time on the edge devices using Python, Pandas and Docker. It is deprecated as we switched to [node-red] and only published for reference.
- **/docs** contains the entire documentation and API specifications for all components including all information to buy, assemble and setup the hardware
