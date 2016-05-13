USSH
====

Description
===========
Ussh searches for servers using uptime.  If you select a server
from the menu it logs you into that server.

Install
=======

    git clone http://github.com/cswank/ussh
    cd ussh
    go get
    go install

Usage
=====

In order to use it you must set

    export UPTIME_ADDR=https://uptime.ops.sendgrid.net/api
    export UPTIME_KEY=<the uptime secret key (see https://uptime.ops.sendgrid.net to get the key)>
    export UPTIME_USER=<your ldap username>


Then type, for example

    ussh sender

A menu will pop up:

    ┌Results────────────────────────────────────┐
    │[1] senderidentity0001s1mdw1.sendgrid.net  │
    │[2] senderidentity0001p1mdw1.sendgrid.net  │
    │[3] senderidentity0002s1mdw1.sendgrid.net  │
    │[4] senderidentity0002p1mdw1.sendgrid.net  │
    └───────────────────────────────────────────┘

Type the number you want to ssh into and you have a session.  Exit
the session as normal (C-d or exit).

In order to quit without logging into anything type control-d.


