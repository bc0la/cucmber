# CUCMber
Steal config files from cisco phones/cucm 

Inspired by https://github.com/trustedsec/SeeYouCM-Thief

Takes an input file of Cisco phone IPs (harvest from gowitness, etc) and attempts to pull tftp address and hostname. At the moment, it dumps all found config files in ./output/ for parsing. 

Consider `grep -i password` or userID

I'm not sure which models are supported, sorry!
