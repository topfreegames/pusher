import string
import random
from sys import argv

def id_generator(size=6, chars=string.ascii_uppercase + string.digits):
    return ''.join(random.choice(chars) for _ in range(size))

def random_message(service):
    if service == "apns":
        return '{"DeviceToken":"'+id_generator(size=64)+'","Payload":{"aps":{"alert":"Helena miss you! come play!"}},"push_expiry":0, "metadata": {"jobId": "86edb3c3-6b5e-40dc-9f14-4ba831daf87c"}}'
    else:
        return '{"error":"DEVICE_UNREGISTERED","to": "'+id_generator(size=152)+'", "notification": {"title": "Come play!", "body": "Helena miss you! come play!"}, "dry_run": true, "metadata": {"jobId": "77372c1e-c124-4552-b77a-f4775bbad850"}}'


script, filename = argv

print "We're going to erase %r." % filename
print "If you don't want that, hit CTRL-C (^C)."
print "If you do want that, hit RETURN."

raw_input("?")

print "Opening the file..."
target = open(filename, 'w')

target.truncate()

print "Do you want to generate apns or gcm pushes?"

service = raw_input("")

print "How many messages do you want to generate?"

num_messages = raw_input("")

print "I'm going to write fake messages to the file."

for i in range(0, int(num_messages)):
    target.write(random_message(service))
    target.write("\n")

print "Done."
target.close()
