using System;
using Newtonsoft.Json;

namespace SampleNugetProject
{
    class Program
    {
        static void Main(string[] args)
        {
            var obj = new { Name = "Hello", Value = 42 };
            Console.WriteLine(JsonConvert.SerializeObject(obj));
        }
    }
}
