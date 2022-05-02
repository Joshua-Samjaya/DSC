import unittest
from command import *

class TestSum(unittest.TestCase):

    def test_sum(self):
        make("file1")
        self.assertTrue(is_exist("file1"))
        delete("file1")

    def test_sum_tuple(self):
        make("file1")
        delete("file1")
        self.assertFalse(is_exist("file1"))

    
if __name__ == '__main__':
    unittest.main()