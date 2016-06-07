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

    export USSH_USER=<your ldap username>

Also, this will read your ~/.chef/knife.rb file.  However, it won't
do the ruby string substitutions, so you will have to change

    client_key "#{home_dir}/.chef/sendgrid.pem"

to
    
    client_key "/Users/<username>/.chef/sendgrid.pem"

If you don't like the colors you can play witb the three
that are used by setting, for example:

    export USSH_COLOR_1=blue
    export USSH_COLOR_2=red
    export USSH_COLOR_2=magenta

The choices are black, red, green, yellow, blue, magenta,
cyan, and white.
  
Then type, for example

    ussh sender

A menu will pop up:

    hosts
       senderidentity0001s1mdw1.sendgrid.net
       senderidentity0002s1mdw1.sendgrid.net
       senderidentity0001p1mdw1.sendgrid.net
       senderidentity0002p1mdw1.sendgrid.net
    filter

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


