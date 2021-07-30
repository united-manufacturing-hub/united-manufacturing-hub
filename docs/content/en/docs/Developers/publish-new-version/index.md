---
title: "How to publish a new version"
linktitle: "How to publish a new version"
description: >
  The UMH uses the semantic versioning. This article explains how to increase the version number and what steps are needed to take
---


1. Ensure mergability from staging to main (e.g. git rebase)
2. Update the helm charts `factorycube-server` and `factorycube-edge` by going into `Charts.yaml` and changing the version to the next version
3. 
```
helm package ../factorycube-server/
helm package ../factorycube-edge/
```
3. Go into `deployment/helm-repo` and execute `helm repo index --url https://repo.umh.app --merge index.yaml .`
4. Commit the change
5. Merge PR from staging to main
6. Add a new release containing a changelog of all changes
