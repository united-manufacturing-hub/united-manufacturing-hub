"""
This file provides yaml linting and lint reporting
"""
import json
import os.path
import re
from pathlib import Path

from yamllint import linter
from yamllint.config import YamlLintConfig

from .git import Git
from .helper import Progressbar
from .ilib import LibInterface
from .log import Colors
from .log import Log


class LibYamlLint(LibInterface):
    """
    This class executes yamllint on changed files and reports its findings
    """
    config: YamlLintConfig
    yaml_files: list
    allowed_lints: list[str]
    warn_lints: list[str]
    lintstr_regex: re.Pattern
    lints: dict

    def __init__(self):
        """
        Initializes config parameters and gets changed files
        """
        self.config = YamlLintConfig('extends: default')
        self.lintstr_regex = re.compile(r"\([\w|-]+\)$")
        self.lints = dict()

        # Check if current branch has upstream
        # If so, only include changed yaml files
        # If not check all yaml files
        self.yaml_files = []
        if Git.has_upstream():
            changes = Git.get_committed_changes()
            for change in changes:
                if change.endswith(".yaml"):
                    path = f"{Git.get_repository_root()}/{change}"
                    if not os.path.isfile(path):
                        Log.warn(f"Skipping non-existing file {path}")
                    else:
                        self.yaml_files.append(path)
        else:
            files = list(Path(Git.get_repository_root()).rglob('*.yaml'))
            for path in files:
                if not os.path.isfile(path):
                    Log.warn(f"Skipping non-existing file {path}")
                else:
                    self.yaml_files.append(path)
        self.yaml_files = list(dict.fromkeys(self.yaml_files))

        # Loads config to allow some lint rules
        with open(f"{Git.get_repository_root()}/.githooks/config.json") as cfg_file:
            cfg = json.load(cfg_file)
            self.allowed_lints = cfg["yamllint"]["allow"]
            self.warn_lints = cfg["yamllint"]["warn"]

    def get_lint_type_from_str(self, lintstr: str) -> str:
        """
        :param lintstr: Output lint from yamllint
        :return: lint rule (https://yamllint.readthedocs.io/en/stable/rules.html)
        """
        _type = self.lintstr_regex.findall(lintstr)
        if len(_type) != 1:
            Log.warn(f"Failed to find lint type for '{lintstr}'")
        _type = _type[0].replace("(", "").replace(")", "")
        return _type

    def check(self):
        """
        Applies yamllint to all yaml files that should be checked
        and parses the output into an dict easier handling
        :return:
        """
        ly = len(self.yaml_files)
        if ly == 0:
            Log.info("No yaml files to check")
            return
        Log.info(f"Checking {ly} yaml files")

        pb = Progressbar(ly)

        for path in self.yaml_files:
            self.lints[path] = dict()
            self.lints[path]["warn"] = dict()
            self.lints[path]["error"] = dict()
            with open(path, "r") as yfile:
                result = list(linter.run(yfile, self.config))
                for lint in result:
                    lint = str(lint)
                    lint_type = self.get_lint_type_from_str(lint)
                    lint = lint.replace(lint_type, "")[:-3]
                    if lint_type in self.allowed_lints:
                        continue
                    if lint_type in self.warn_lints:
                        if lint_type not in self.lints[path]["warn"]:
                            self.lints[path]["warn"][lint_type] = []
                        self.lints[path]["warn"][lint_type].append(lint)
                    else:
                        if lint_type not in self.lints[path]["error"]:
                            self.lints[path]["error"][lint_type] = []
                        self.lints[path]["error"][lint_type].append(lint)
            pb.add_progress()
        pb.finish()

    def report(self) -> int:
        """
        Prints results of lint checking in an formatted way.
        :return: number of errors found
        """
        if len(self.yaml_files) == 0:
            return 0
        errors = 0
        warnings = 0
        for path, lint in self.lints.items():
            Log.info(f"{path}")
            for lint_type, lint_messages in lint["error"].items():
                Log.fail(f"\t{lint_type}")
                for lint_message in lint_messages:
                    Log.fail(f"\t\t{lint_message}")
                    errors += 1

            for lint_type, lint_messages in lint["warn"].items():
                Log.warn(f"\t{lint_type}")
                for lint_message in lint_messages:
                    Log.warn(f"\t\t{lint_message}")
                    warnings += 1

        print()
        if errors > 0:
            failstr = f"|| Yaml lint failed with {errors} errors and {Colors.WARNING}{warnings} warnings{Colors.ENDC}{Colors.FAIL} ||"
            fstrlen = len(failstr) - 14
            Log.fail('=' * fstrlen)
            Log.fail(failstr)
            Log.fail('=' * fstrlen)

        elif warnings > 0:
            warnstr = f"Yaml lint succeeded with {warnings} warnings"
            Log.warn('=' * len(warnstr))
            Log.warn(warnstr)
            Log.warn('=' * len(warnstr))
        else:
            Log.ok("====================")
            Log.ok(f"Yaml lint succeeded")
            Log.ok("====================")

        return errors

    def run(self):
        """
        Runs check & report
        :return: reports return value
        """
        self.check()
        return self.report()
