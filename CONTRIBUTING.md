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

Internally we work in 2 week sprints. The list of all open issues without an assigned milestone is our backlog. At the beginning of each sprint we take the most important issues and assign them a milestone. Thus we define the content of a Sprint. Each sprint / milestone results in a new version of the United Manufacturing Hub. Therefore, milestones are named and versioned according to the [structure further down](#versioning). Bigger features, so called EPICs, can be divided into multiple issues and get's its own MINOR version. This can also result in one issue beeing added to two milestones: one for the current sprint and one for the corresponding EPIC and MINOR version.

At the beginning of the sprint a draft pull request is created from staging to main.

#### Processing of the work package

When work on a new work package (issue) is started, a new branch is created from staging with the following name: `issueID-short-description`, e.g. `465-improve-documentation-factorycube`. When the work package is processed and tested, a pull request is created from the branch to staging. A second person reviews the code and automatic tests are performed. If everything is ok the feature is committed to staging using squash & merge. The commit should follow the following naming: `issueID Full issue description`. This closes the issue.

#### Sprint end

After 1.5 weeks the sprint should be finished with all work packages. If not, the remaining ones will be put back into the backlog. The remaining 2 days are used to test the code in the staging environment. If all tests are ok and all open bugs have been fixed, the pull request is approved and staging is merged with main. The next sprint is then discussed.

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
