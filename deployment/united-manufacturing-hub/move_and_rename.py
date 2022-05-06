import os
import shutil


def main():
    directory = "templates"
    for filename in os.listdir(directory):
        f = os.path.join(directory, filename)
        if os.path.isfile(f):
            if "NOTES" in filename or ".tpl" in filename or "ServiceAccount" in filename:
                continue

            oldpath_wo_extension = os.path.splitext(filename)[0]
            # print(filename)

            disallowed = ["-deployment", "-pvc", "-pdb", "-statefulset", "-ingress", "-config-secret", "-secrets",
                          "-persistentvolumeclaim", "-secret", "-configmap", "-local-service", "-hpa", "-service",
                          "-flows"]

            newpath = oldpath_wo_extension.lower()
            for dis in disallowed:
                newpath = newpath.replace(dis, "")
            newpathX = os.path.join(directory, newpath)
            os.makedirs(newpathX, exist_ok=True)

            newname = filename.replace(newpath, "").replace("-", "").lower()
            print("{} -> {}\\{}".format(f, newpathX, newname))

            shutil.copy(f, os.path.join(newpathX, newname))


if __name__ == "__main__":
    main()
