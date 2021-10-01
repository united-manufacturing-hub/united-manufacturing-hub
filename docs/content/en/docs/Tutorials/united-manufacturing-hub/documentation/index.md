---
title: "Setting up the documentation"
linkTitle: "Setting up the documentation"
aliases:
    - /docs/tutorials/documentation/
weight: 1
description: >
  This document explains how to get started with the documentation locally
---

1. Clone the repo 
2. Go to /docs and execute `git submodule update --init --recursive` to download all submodules
3. `git init && git add . && git commit -m "test"` (yes it is quite stupid, but it works)
4. Startup the development server by using `sudo docker-compose up --build`
