---
title: "How to publish a new version"
linktitle: "How to publish a new version"
description: >
  The UMH uses the semantic versioning. This article explains how to increase the version number and what steps are needed to take
---

1. Create release branch (e.g., v0.6.0) from staging. Docker will automatically build Docker containers with the tag `VERSION-prerelease`, e.g., `v0.6.0-prerelease`
2. Create PR from release branch to main
3. Update the helm charts `united-manufacturing-hub` by going into `Charts.yaml` and changing the version to the next version including a `-prerelease`
4. Adjust repo link `https://repo.umh.app` in `docs/static/examples/development.yaml` to the deploy-preview of helm-repo, e.g., `https://deploy-preview-515--umh-helm-repo.netlify.app/` with 515 beeing the PR created in 2. Additionally, add a --devel flag to both helm install commands, so that helm considers the prerelease as a valid version.
5. Go into the folder `deployment/helm-repo` and execute
```
helm package ../united-manufacturing-hub/
```
6. Go into `deployment/helm-repo` and execute `helm repo index --url https://deploy-preview-515--umh-helm-repo.netlify.app/ --merge index.yaml .`
7. Commit changes. Wait for all container and deploy previews to be created. Conduct test on K300 by specifying the new cloud-init file e.g., `https://deploy-preview-515--umh-docs.netlify.app/examples/development.yaml` (you can create a bit.ly link for that)
8. Test
9. Conduct steps 3 - 6 with changed version v0.6.0 (instead of v0.6.0-prerelease) and changed repo index url: https://repo.umh.app
10. Execute `npx semantic-release --branches "master" --branches "v0.6.0" --plugins "@semantic-release/commit-analyzer" --plugins "@semantic-release/release-notes-generator" --plugins "@semantic-release/changelog"`
11. Remove old helm packages from prerelease from repo
12. Merge PR from staging to main
13. Add a new release containing a changelog of all changes

