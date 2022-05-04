import subprocess
import os
import shutil

    
def make(filename):
    os.makedirs(filename)

def delete(filename):
    shutil.rmtree(filename)

def is_exist(filename):
    return os.path.exists(filename)


