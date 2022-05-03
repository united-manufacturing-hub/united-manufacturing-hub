"""
This file provides docker file linting and tries to build them
"""

import os.path
import re
import subprocess
from pathlib import Path
from threading import Thread

from .git import Git
from .helper import Progressbar
from .ilib import LibInterface
from .log import Log


class LibDockerLint(LibInterface):
    projects = []
    build_outcomes = []

    def __init__(self, force):
        # Check if current branch has upstream
        # If so, only check changed projects
        # If not check all projects

        go_projects = []

        if Git.has_upstream() and not force:
            changes = Git.get_committed_changes()
            for change in changes:
                path = f"{Git.get_repository_root()}/{change}"
                if change.endswith(".go"):
                    if not os.path.isfile(path):
                        Log.warn(f"Skipping non-existing file {path}")
                    else:
                        xpath = os.path.dirname(os.path.abspath(path))
                        matches = re.search(r"golang\\cmd\\([\w|-]+)", xpath)
                        if matches is not None:
                            go_projects.append(matches.group(1))
        else:
            go_files = list(Path(Git.get_repository_root()).rglob('*.go'))
            for path in go_files:
                if not os.path.isfile(path):
                    Log.warn(f"Skipping non-existing file {path}")
                else:
                    xpath = os.path.dirname(os.path.abspath(path))
                    matches = re.search(r"golang\\cmd\\([\w|-]+)", xpath)
                    if matches is not None:
                        go_projects.append(matches.group(1))

        go_projects = list(dict.fromkeys(go_projects))
        self.projects.extend(go_projects)

    def check_single(self, project, repo_root, pb):
        docker_file_path = f"{repo_root}/deployment/{project}/Dockerfile"

        if not os.path.isfile(docker_file_path):
            pb.add_progress()
            return

        p = subprocess.Popen(['docker', 'build', '-f', docker_file_path, '.'], stdin=subprocess.PIPE,
                             stdout=subprocess.PIPE, stderr=subprocess.PIPE, cwd=repo_root)
        output, err = p.communicate()
        rc = p.returncode

        self.build_outcomes.append(
            {
                "name": project,
                "path": docker_file_path,
                "rc": rc,
                "err": err
            }
        )
        pb.add_progress()

    def check(self):
        repo_root = Git.get_repository_root()
        ly = len(self.projects)
        if ly == 0:
            Log.info("No docker projects to check")
            return

        Log.info(f"Checking {ly} docker projects")

        pb = Progressbar(ly)

        threads = []
        for project in self.projects:
            t = Thread(target=self.check_single, args=(project, repo_root, pb))
            t.start()
            threads.append(t)

        for t in threads:
            t.join()

        pb.finish()

    def report(self):
        if len(self.build_outcomes) == 0:
            return 0
        errors = 0
        for outcomes in self.build_outcomes:
            if outcomes["rc"] != 0:
                Log.info(f"{outcomes['name']}")
                err = outcomes['err'].decode("utf-8")
                for line in err.splitlines():
                    Log.fail(f"\t{line}")
                errors += 1

        if errors > 0:
            print()
            failstr = f"|| Docker lint failed with {errors} errors ||"
            fstrlen = len(failstr)
            Log.fail('=' * fstrlen)
            Log.fail(failstr)
            Log.fail('=' * fstrlen)
        else:
            Log.ok("======================")
            Log.ok(f"Docker lint succeeded")
            Log.ok("======================")

        return errors

    def run(self):
        """
        Runs check & report
        :param force:
        :return: reports return value
        """
        self.check()
        return self.report()
