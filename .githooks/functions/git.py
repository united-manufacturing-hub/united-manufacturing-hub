"""
This file provides git related utilities
"""
import subprocess


def memoize(function):
    """
    Runs a function once and caches it's return values for the rest of the runtime
    :param function: function to execute once
    :return: function return value (either by executing it or returning the cached values)
    """
    memo = {}

    def wrapper(*args):
        if args in memo:
            return memo[args]
        else:
            rv = function(*args)
            memo[args] = rv
            return rv

    return wrapper


class Git:
    """
    Provides git wrapper methods
    """
    @staticmethod
    @memoize
    def get_current_branch() -> str:
        """
        Gets the name of the current branch
        :return: Branch name
        """
        output = subprocess.check_output(['git', 'rev-parse', '--abbrev-ref', 'HEAD']).decode("UTF-8").strip()
        return output

    @staticmethod
    @memoize
    def has_upstream() -> bool:
        """
        Checks if current branch exists on upstream
        :return: Upstream exists
        """
        remote_branches = subprocess.check_output(['git', 'branch', '-r']).decode("UTF-8").strip()
        current_branch = Git.get_current_branch()
        return current_branch in remote_branches

    @staticmethod
    @memoize
    def get_committed_changes() -> [str]:
        """
        Returns list of committed files
        :return: Committed files
        """
        if not Git.has_upstream():
            return []
        current_branch = Git.get_current_branch()
        changes_str = subprocess.check_output(
            ['git', 'log', '--name-only', '--pretty=format:', f"origin/{current_branch}..HEAD"]).decode("UTF-8").strip()

        return list(dict.fromkeys(changes_str.splitlines()))

    @staticmethod
    @memoize
    def get_repository_root() -> str:
        """
        Returns root path of repository
        :return: repository root path
        """
        return subprocess.check_output(['git', 'rev-parse', '--show-toplevel']).decode("UTF-8").strip()
