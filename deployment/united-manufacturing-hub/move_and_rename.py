# Copyright 2023 UMH Systems GmbH
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
import shutil


def main():
    """
    Moves helm templates into subdirectories based on the templates name.
    :return:
    """
    directory = "templates"
    for filename in os.listdir(directory):
        f = os.path.join(directory, filename)
        if os.path.isfile(f):
            if "NOTES" in filename or ".tpl" in filename or "ServiceAccount" in filename:
                continue

            old_file_name_without_extension = os.path.splitext(filename)[0]

            type_endings = ["-deployment", "-pvc", "-pdb", "-statefulset", "-ingress", "-config-secret", "-secrets",
                          "-persistentvolumeclaim", "-secret", "-configmap", "-local-service", "-hpa", "-service",
                          "-flows"]

            new_file_name = old_file_name_without_extension.lower()
            for dis in type_endings:
                new_file_name = new_file_name.replace(dis, "")
            new_path = os.path.join(directory, new_file_name)
            os.makedirs(new_path, exist_ok=True)

            newname = filename.replace(new_file_name, "").replace("-", "").lower()
            print("{} -> {}\\{}".format(f, new_path, newname))

            shutil.copy(f, os.path.join(new_path, newname))


if __name__ == "__main__":
    main()
