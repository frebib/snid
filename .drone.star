repo = "frebib/snid"
branches = ["metrics"]
architectures = ["amd64", "arm64"]


def main(ctx):
  builds = []
  depends_on = []

  for arch in architectures:
    key = "build-%s" % arch
    builds.append(step(arch, key))
    depends_on.append(key)

  if ctx.build.branch in branches:
    builds.append(publish(depends_on))

  return builds


def step(arch, key):
  return {
    "kind": "pipeline",
    "name": key,
    "platform": {
      "os": "linux",
      "arch": arch,
    },
    "steps": [
      {
        "name": "build",
        "image": "registry.spritsail.io/spritsail/docker-build",
        "pull": "always",
      },
      {
        "name": "publish",
        "pull": "always",
        "image": "registry.spritsail.io/spritsail/docker-publish",
        "settings": {
          "registry": {"from_secret": "registry_url"},
          "login": {"from_secret": "registry_login"},
        },
        "when": {
          "branch": branches,
          "event": ["push"],
        },
      },
    ],
  }


def publish(depends_on):
  return {
    "kind": "pipeline",
    "name": "publish-manifest",
    "depends_on": depends_on,
    "platform": {
      "os": "linux",
    },
    "steps": [
      {
        "name": "publish",
        "image": "registry.spritsail.io/spritsail/docker-multiarch-publish",
        "pull": "always",
        "settings": {
          "tags": [
            "metrics",
          ],
          "src_registry": {"from_secret": "registry_url"},
          "src_login": {"from_secret": "registry_login"},
          "dest_registry": "registry.spritsail.io",
          "dest_repo": repo,
          "dest_login": {"from_secret": "spritsail_login"},
        },
        "when": {
          "branch": branches,
          "event": ["push"],
        },
      },
    ],
  }


# vim: ft=python sw=2
