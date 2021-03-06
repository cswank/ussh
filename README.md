USSH
====

Description
===========
Ussh searches for servers using the chef api.  If you select a server
from the menu it logs you into that server.

Install
=======

    go get github.com/cswank/ussh
    
Usage
=====

In order to use it you must set

    export USSH_USER=<your ldap username>

Also, this will read your ~/.chef/knife.rb file.  However, it won't
do the ruby string substitutions, so you will have to change

    client_key "#{home_dir}/.chef/somepem.pem"

to
    
    client_key "/Users/<username>/.chef/somepem.pem"

If you don't like the colors you can play witb the three
that are used by setting, for example:

    export USSH_COLOR_1=blue
    export USSH_COLOR_2=red
    export USSH_COLOR_3=magenta

The choices are black, red, green, yellow, blue, magenta,
cyan, and white.

By default, 20 results are printed to the screen (you can scroll
to all of the results).  If you want to see more by default:

    export USSH_WINDOW=<some int>.
  
Then type, for example

    ussh server

Type 'h' to see the help screen:

<img src="./docs/images/help.png" width="620"/>

Type 'q' to exit the help screen.

A menu will pop up that will contain all nodes with the word
'server' in their name.

<img src="./docs/images/screenshot1.png" width="620"/>

Type 'p', 'n' (emacs style) or use the up and down arrows to
highlight a different node.

<img src="./docs/images/screenshot2.png" width="620"/>

Hit the enter key to ssh to the highlighted node.  You can
select multiple nodes and cssh into all of them by using the
space bar to select.  So, to ssh into server2 and server 4 you
navigate to server1, hit space, navigate to server 4 and hit enter.

<img src="./docs/images/screenshot3.png" width="620"/>

Another way to cssh to multiple nodes is to type C-a.  A cssh session
will then ssh into all visible nodes whether they are highlighted or
not.

You can also filter the result list down in a few ways.  One is to
type Control-f (C-f).  The cursor will move to the filter box.  After
you are done typing a filter term hit enter to move the cursor back to
the node list.

<img src="./docs/images/screenshot4.png" width="620"/>

Search terms can be separated by a comma.  The filter terms will be
ANDed together.

<img src="./docs/images/screenshot5.png" width="620"/>

You can also pass a filter string in when you start the app:

    ussh server -f .com

Another way to get a more refined list of nodes is to use a --role
argument when starting ussh (chef role, that is):

    ussh server --role teamA

You can type 'c' to copy the current host to your clipboard.  Also, you
can type 'C' to copy the current host to your clipboard with USSH_USER@
prepended.

In order to quit without logging into anything type control-d.


