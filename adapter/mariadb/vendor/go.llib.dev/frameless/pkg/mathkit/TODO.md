```
--- FAIL: TestBigInt (0.00s)
    --- FAIL: TestBigInt/#Sub (0.00s)
        --- FAIL: TestBigInt/#Sub/when_substractions_result_would_yield_a_big_int_then_result_will_be_equalement_of_the_sum_of_the_values (0.00s)
            mathkit_test.go:651: [Equal]
            mathkit_test.go:651:

                /* mathkit.BigInt[int] */ "-18446744073709551616"  |  /* mathkit.BigInt[int] */ "0"
    --- FAIL: TestBigInt/#Values (0.00s)
        --- FAIL: TestBigInt/#Values/when_the_big_int_value_is_positive_then_the_iterated_values_sum_equal_to_the_big_int_itself (0.00s)
            mathkit_test.go:798: [Equal]
            mathkit_test.go:798:

                /* mathkit.BigInt[int] */ "107521203160998449967"  |  /* mathkit.BigInt[int] */ "0"
        --- FAIL: TestBigInt/#Values/when_the_big_int_value_is_negative_then_the_iterated_values_sum_equal_to_the_big_int_itself (0.00s)
            mathkit_test.go:815: [Equal]
            mathkit_test.go:815:

                /* mathkit.BigInt[int] */ "-100832227978526028885"  |  /* mathkit.BigInt[int] */ "0"


        	 	  describe #Sub
        	 	    when substraction's result would yield a big int
        	 	      then result will be equalement of the sum of the values [FAIL]
        	 	  describe #Values
        	 	    when the big int value is positive
        	 	      then the iterated value's sum equal to the big int itself [FAIL]
        	 	    when the big int value is negative
        	 	      then the iterated value's sum equal to the big int itself [FAIL]


        	 	 TESTCASE_SEED=9146736256503637731
```

