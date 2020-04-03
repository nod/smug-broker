
```
 ___ _ __ ___  _   _  __ _ 
/ __| '_ ` _ \| | | |/ _` |
\__ \ | | | | | |_| | (_| |
|___/_| |_| |_|\__,_|\__, |
                     |___/ 
```

# smug

Want to mirror everything from one irc to another with ease?

Want to proxy everything from a slack channel to an irc channel?

Want to proxy everything from a slack channel to an external RESTful api?

Broker communications between irc, slack, other services.

# quickstart

To connect a slack channel to irc, set some environment variables and then run
the docker command.

```
export SMUG_IRC_SERVER="irc.example.com:6667"
export SMUG_IRC_CHANNEL="#my_chan"
export SMUG_SLACK_TOKEN="xoxo-blah"
docker run -e SMUG_IRC_SERVER,SMUG_IRC_CHANNEL,SMUG_SLACK_TOKEN \
  nod/smug-broker:latest
```

Note - slack has complicated getting the proper token.
See [doc/slack.md](doc/slack.md) for brief instructions.

## SLACK PERMISSIONS

When you create an app integration in slack, be sure to add the `users:read`
scope or you will have issues.

I'm trying to automate this process in an upcoming release but for now it's a
manual step.

# usage

Download a release or build as documented below.

Copy the `smug.yaml.template` file to `smug.yaml`.  Edit the file to set your
connection parameters and keys.

Run like the following:

```bash
$ ./smug -conf=smug.yaml
```

See `doc/config.md` for more configuration details.

# building

Assuming golang version 1.12+ installed, you can do the following:

```bash
$ cd $GOPATH/src
$ git clone https://github.com/nod/smug
$ cd smug
$ make test
$ make
```

If all goes well, this should create a `build/smug` as a compiled binary at that
point.

