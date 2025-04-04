# CUCMber
Steal config files from cisco phones/cucm 

Inspired by https://github.com/trustedsec/SeeYouCM-Thief

Takes an input file of Cisco phone IPs (harvest from gowitness, etc) and attempts to pull tftp address and hostname. At the moment, it dumps all found config files in ./output/ for parsing. 

Consider `grep -i password` or userID for user enum

I like:
`cat * | grep -ia password | grep -v word\>\<\/ | grep -i pass`

I'm not sure which models are supported, sorry!

## NOTE:

If you have issues with the main branch, try the refactor branch, has some older code. The main branch has been modified to run faster and provide (hopefully) a more thorough search, ConfigFileCacheList.txt has not yet been tested.
