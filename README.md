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
    godep go install

Usage
=====

In order to use it you must set

    export UPTIME_ADDR=https://uptime.ops.sendgrid.net/api
    export UPTIME_KEY=<the uptime secret key (see https://uptime.ops.sendgrid.net to get the key)>
    export UPTIME_USER=<your ldap username>

Optional, if you use a light background terminal:
    export UPTIME_THEME=light

Then type, for example

    ussh sender

A menu will pop up:

    ┌sesults (C-d to exit)─────────────────────┐
    │1  senderidentity0001s1mdw1.sendgrid.net  │
    │2  senderidentity0001p1mdw1.sendgrid.net  │
    │3  senderidentity0002s1mdw1.sendgrid.net  │
    │4  senderidentity0002p1mdw1.sendgrid.net  │
    └──────────────────────────────────────────┘
    ┌filter (C-f)───────────────────────────┐
    │                                       │
    └───────────────────────────────────────┘
    ┌ssh to (enter number)──────────────────┐
    │                                       │
    └───────────────────────────────────────┘

Type control-f to filter the results down:

    ┌sesults (C-d to exit)─────────────────────┐
    │1  senderidentity0001p1mdw1.sendgrid.net  │
    │2  senderidentity0002p1mdw1.sendgrid.net  │
    │                                          │
    │                                          │
    └──────────────────────────────────────────┘
    ┌Filter (C-f)───────────────────────────┐
    │p1                                     │
    └───────────────────────────────────────┘
    ┌ssh to (enter number)──────────────────┐
    │                                       │
    └───────────────────────────────────────┘

Type the number you want to ssh into (and hit enter) and you have a session.
Exit the session as normal (control-d or exit).

In order to quit without logging into anything type control-d.


