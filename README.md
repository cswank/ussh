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

    <img src="https://raw.githubusercontent.com/cswank/ussh/master/docs/images/screenshot1.png" width="100"/>


In order to quit without logging into anything type control-d.


