# CUCMber
Steal config files from cisco phones/cucm 

Inspired by https://github.com/trustedsec/SeeYouCM-Thief

Takes an input file of Cisco phone IPs (harvest from gowitness, etc) and attempts to pull tftp address and hostname. At the moment, it dumps all found config files in ./output/ for parsing. 

Consider `grep -i password` or userID

I'm not sure which models are supported, sorry!


TODO:

- Attempt to pull file as described here: https://nvd.nist.gov/vuln/detail/cve-2013-7030
  - "Working" in refactor branch for now


## NOTE:

very much a work in progress, I am making and breaking things to clean this up in other branches that may or may not be working. feedback welcome! 
