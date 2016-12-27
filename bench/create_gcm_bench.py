import string
import random
from sys import argv

def id_generator(size=6, chars=string.ascii_uppercase + string.digits):
    return ''.join(random.choice(chars) for _ in range(size))

def random_message():
    return '{"to": "'+id_generator(size=152)+'", "notification": {"title": "Come play!", "body": "Helena miss you! come play!"}, "dry_run": true}'

script, filename = argv

print "We're going to erase %r." % filename
print "If you don't want that, hit CTRL-C (^C)."
print "If you do want that, hit RETURN."

raw_input("?")

print "Opening the file..."
target = open(filename, 'w')

target.truncate()

print "How many messages do you want to generate?"

num_messages = raw_input("")

print "I'm going to write fake messages to the file."

for i in range(0, int(num_messages)):
    target.write(random_message())
    target.write("\n")

print "Done."
target.close()
