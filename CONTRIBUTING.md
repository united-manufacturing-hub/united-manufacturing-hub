# Contributing

## Contents

## Introduction

Contributions are what make the open source community such an amazing place to be learn, inspire, and create. Any contributions you make are **greatly appreciated**. In this document we explain how one can contribute to the project. Furthermore, it explains how we work together in general. We are trying to create an atmosphere were we all can collaborate while enjoying programming. See also `CODE_OF_CONDUCT.md` for more information.

## General workflow

### Suggest new functionalities or report bugs

We are happy about every new idea, change request or found bug! For each feature or bug, you can create a Github issue explaining it in more detail. The better explained, the sooner we can get started.

### Internal development process

#### Wording

- With issue we mean the Github Issue functionality
- With milestone we mean the Github milestone functionality
- with sprint we mean a sprint according to the agile process

#### Sprint Start

Internally we work in 2 week sprints. The list of all open issues without an assigned milestone is our backlog.

At the beginning of each sprint we take the most important issues and add them to the project board "current sprint". Thus we define the content of a sprint. Each sprint results in a new version of the United Manufacturing Hub with a increased PATCH version, which includes all smaller changes that are not related to another milestone / bigger epic. For more information about versioning take a look [into the corresponding chapter](#versioning)

An epic consists out of multiple issues that are bundled together in a user story. They can can be divided into multiple issues and get's its own MINOR or MAJOR version.

#### Processing of the work package

When work on a new work package (issue) is started, a new branch is created from staging with the following name: 

`issueID-short-description`, e.g. `465-improve-documentation-factorycube`. 

When the work package is processed and tested, a pull request is created from the branch to staging. A second person reviews the code and automatic tests are performed. If everything is ok the feature is committed to staging using squash & merge. 

The commit should follow the [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) logic. 

This closes the issue.

#### Sprint end

After 1.5 weeks the sprint should be finished with all work packages. If not, the remaining ones will be put back into the backlog. The remaining 2 days are used to test the code in the staging environment. If all tests are ok and all open bugs have been fixed, a release branch is created in the format `release/v0.0.0`. Final tests are conducted. For publishing a new version, see also [our guide on how to publish a new release](https://docs.umh.app/docs/developers/publish-new-version/)The next sprint is then discussed.

### Special features for externals

If you are external and would like to help out, there are a few more notes for you to keep in mind:

- Discuss the work packages you want to tackle with us in advance. This way we can give you input in time and avoid unnecessary work.
- Before we can accept your code, you have to accept the CLA. You can find them in the main directory `CONTRIBUTOR_LICENSE_AGREEMENT_ENTITY.md` and `CONTRIBUTOR_LICENSE_AGREEMENT_INDIVIDUAL.md`.
- Since you cannot create a new branch as an external, fork the project code first and then create a pull request from your fork to staging this project.

## Versioning

Given a version number MAJOR.MINOR.PATCH, increment the:

MAJOR version when you make incompatible API changes,
MINOR version when you add functionality in a backwards compatible manner, and
PATCH version when you make backwards compatible bug fixes.

For every sprint we increase the PATCH version.

This is based on [Semantic Versioning](https://semver.org/).

## Git hooks

### Requirements
 - Python 3.8 or newer
 - Go 1.17 or newer
 - [Yamllint](https://github.com/adrienverge/yamllint#installation)
   - ```pip install yamllint```
 - [Helm](https://helm.sh/docs/intro/install/)
 - [Staticcheck](https://staticcheck.io/docs/getting-started/)

Afterwards set the git hook path for this repository by opening your favorite terminal and executing:

```git config --local core.hooksPath .githooks/```

If you are on linux (or use the Git Bash under Windows), you also need to execute.

```chmod +x .githooks/pre-push```