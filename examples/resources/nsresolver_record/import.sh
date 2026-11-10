# A record is imported by "<zone>/<record id>". Find ids with the nsresolver_records data
# source, or the API: curl https://api.nsresolver.com/v1/example.com/ -H "NSToken: $NSRESOLVER_TOKEN"
terraform import nsresolver_record.www example.com/1234
